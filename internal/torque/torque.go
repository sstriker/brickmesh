// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package torque follows torque through a gear train and reports tooth loads.
//
// The propagation is exact: it is the ratio, the efficiency, and the pitch
// radius, and none of those are in doubt. The FAILURE LIMITS ARE NOT. They are
// community and estimated figures that could not be traced to a primary source,
// every one of them is marked as unverified, and Notice lists them so that no
// report can quietly imply they are measurements. Replace them with your own
// before trusting a PASS.
//
// Kept as a separate concern from internal/mech because it answers a different
// question. Mech asks whether a train can turn; this asks whether it survives
// being turned.
package torque

import (
	"fmt"
	"sort"
)

// Limit is one figure with its provenance attached, because a limit without
// provenance is a number someone will eventually believe.
type Limit struct {
	Value    float64
	Source   string
	Verified bool
}

// Limits, in Ncm for torque and N for force. Edit these.
var Limits = map[string]map[string]Limit{
	"motor_stall_Ncm": {
		"pu_xl_88014": {40.0, "community figure for PF/PU XL", false},
		"pu_l_88013":  {20.0, "estimate, PF L measured lower", false},
	},
	"axle_torsion_Ncm": {
		"axle_standard": {15.0, "rule of thumb, twists before it breaks", false},
	},
	"gear_tooth_force_N": {
		"gear_8t":   {40.0, "8t is the classic first failure", false},
		"gear_16t+": {90.0, "estimate", false},
	},
	"differential_slip_Ncm": {
		"diff_62821": {25.0, "estimate; measure yours", false},
	},
	"clutch_slip_Ncm": {
		// The white 24t with the friction centre. Community figures put it
		// around here; nobody here has measured one.
		"gear_24t_76019": {20.0, "community figure; measure yours", false},
	},
}

// Efficiency of each kind of mesh. A worm is lossy, and that loss is the price
// of it being self-locking.
var Efficiency = map[string]float64{
	"spur": 0.94, "bevel": 0.90, "worm": 0.45, "diff": 0.90,
}

// Stage is one mesh in the train.
type Stage struct {
	Name        string
	DriverTeeth int
	DrivenTeeth int
	Kind        string // spur (default), bevel, worm, diff
}

// Ratio is how much the driven shaft is slowed, and so how much the torque
// rises.
func (s Stage) Ratio() float64 {
	return float64(s.DrivenTeeth) / float64(s.DriverTeeth)
}

// Eff is the fraction of torque that survives the mesh.
//
// An unstated kind is a spur, which is what internal/mech takes it for too. A
// kind that is stated but unrecognised gets the cautious figure instead: it is
// something this does not know about, and guessing a spur's efficiency for it
// would be optimistic in the direction that hides a failure.
func (s Stage) Eff() float64 {
	if s.Kind == "" {
		return Efficiency["spur"]
	}
	if e, ok := Efficiency[s.Kind]; ok {
		return e
	}
	return 0.9
}

// Row is what one stage does to the torque passing through it.
type Row struct {
	Stage        string
	Ratio        float64
	TorqueInNcm  float64
	TorqueOutNcm float64
	ForceDriverN float64
	ForceDrivenN float64
	DriverTeeth  int
	DrivenTeeth  int
}

// PitchRadiusMM follows from the meshing rule: two gears mesh at
// (t1+t2)/16 studs between centers, so each contributes t/32 studs, and a stud
// is 8 mm.
func PitchRadiusMM(teeth int) float64 { return float64(teeth) / 2.0 }

// ToothForceN is the tangential load on the tooth flank.
func ToothForceN(torqueNcm float64, teeth int) float64 {
	torqueNm := torqueNcm / 100.0
	radiusM := PitchRadiusMM(teeth) / 1000.0
	if radiusM == 0 {
		return 0
	}
	return torqueNm / radiusM
}

// Propagate walks the train, reporting torque and tooth load at every stage.
func Propagate(inputTorqueNcm float64, stages []Stage) []Row {
	t := inputTorqueNcm
	rows := make([]Row, 0, len(stages))
	for _, s := range stages {
		out := t * s.Ratio() * s.Eff()
		rows = append(rows, Row{
			Stage: s.Name, Ratio: s.Ratio(),
			TorqueInNcm: t, TorqueOutNcm: out,
			ForceDriverN: ToothForceN(t, s.DriverTeeth),
			ForceDrivenN: ToothForceN(out, s.DrivenTeeth),
			DriverTeeth:  s.DriverTeeth, DrivenTeeth: s.DrivenTeeth,
		})
		t = out
	}
	return rows
}

// Assessment is one thing the loads have to say.
type Assessment struct {
	Level  string // OK, WARN or FAIL
	Detail string
}

// Assess reports the gears that will skip and the axles that will twist.
func Assess(rows []Row) []Assessment {
	small := Limits["gear_tooth_force_N"]["gear_8t"].Value
	big := Limits["gear_tooth_force_N"]["gear_16t+"].Value
	axle := Limits["axle_torsion_Ncm"]["axle_standard"].Value

	var out []Assessment
	for _, r := range rows {
		for _, side := range []struct {
			teeth int
			force float64
		}{{r.DriverTeeth, r.ForceDriverN}, {r.DrivenTeeth, r.ForceDrivenN}} {
			// The small gears fail first, and the 8t famously so.
			limit := big
			if side.teeth <= 12 {
				limit = small
			}
			switch {
			case side.force > limit:
				out = append(out, Assessment{"FAIL", fmt.Sprintf(
					"%s: the %dt sees %.0f N on a tooth against a limit of %.0f N; "+
						"it will skip", r.Stage, side.teeth, side.force, limit)})
			case side.force > 0.7*limit:
				out = append(out, Assessment{"WARN", fmt.Sprintf(
					"%s: the %dt sees %.0f N on a tooth, %.0f%% of the limit",
					r.Stage, side.teeth, side.force, 100*side.force/limit)})
			}
		}
		if r.TorqueOutNcm > axle {
			out = append(out, Assessment{"WARN", fmt.Sprintf(
				"%s: the axle after it carries %.1f Ncm, above the %.0f Ncm rule "+
					"of thumb. Keep it short and bear it at both ends",
				r.Stage, r.TorqueOutNcm, axle)})
		}
	}
	if len(out) == 0 {
		out = append(out, Assessment{"OK",
			"no limit exceeded — though see what the limits are worth, below"})
	}
	return out
}

// Notice lists every limit that is not a measurement, which at present is all
// of them. Print it with any assessment: a report that hides where its numbers
// came from is worse than no report.
func Notice() []string {
	var groups []string
	for g := range Limits {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	var out []string
	for _, g := range groups {
		var names []string
		for k := range Limits[g] {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if l := Limits[g][k]; !l.Verified {
				out = append(out, fmt.Sprintf("  %s.%s = %g  (%s)",
					g, k, l.Value, l.Source))
			}
		}
	}
	return out
}

// SlipLimitNcm is what a 24-tooth slip clutch is taken to give way at, and where
// that figure comes from.
//
// An estimate, like every other limit here, and the caller is told so in the
// same breath so that no report can quietly imply it was measured.
func SlipLimitNcm() (float64, string) {
	if l, ok := Limits["clutch_slip_Ncm"]["gear_24t_76019"]; ok {
		if l.Verified {
			return l.Value, l.Source
		}
		return l.Value, l.Source + ", unverified"
	}
	return 0, "no figure"
}
