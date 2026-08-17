# Provenance and licenses

This repository does **not** contain the parts libraries. The extractor fetches
them on first use. That is deliberate: the licenses permit use, but the BY-SA
share-alike condition carries over to derived data, and you don't want to pull
that into your own repository unnoticed.

## LDraw Parts Library

Licensed under CCAL 2.0 (Creative Commons Attribution License 2.0). Parts are
contributed by LDraw.org volunteers; each `.dat` file names its own author in
the header.

## LDCad Shadow Library

By Roland Melkert, licensed **CC BY-SA 4.0**. This is the layer that describes,
per part, where the holes and pins are and which way they point.

Mind the share-alike condition: if you publish a file derived from this data —
the generated catalog, for instance — that file falls under the same license.
The code that processes the data does not.

## This code

Apache License 2.0, see `LICENSE` and `NOTICE`.

## No affiliation with the LEGO Group

LEGO® is a trademark of the LEGO Group, which does not sponsor, authorize or
endorse this project.
