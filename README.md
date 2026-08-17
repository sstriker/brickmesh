# brickmesh

Analyze and construct LEGO Technic mechanisms: validate geometry, work out
torque and gear ratios, and export the result as `.ldr`.

## Status

This is the first step: license, provenance attribution, and the ground rules
for what does and does not belong in the repository. The code follows.

## Parts libraries

This repository does **not** contain the LDraw Parts Library or the LDCad Shadow
Library. Both are fetched on first use. That is a deliberate choice: the CC
BY-SA 4.0 share-alike condition carries over to data derived from the shadow
library — the generated catalog, that is — and that should not end up in this
repository unnoticed. Hence `.gitignore` keeps the libraries and `data/` out of
version control.

See [ATTRIBUTION.md](ATTRIBUTION.md) for full provenance and the license terms
of each source.

## License

Apache License 2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

## No affiliation with the LEGO Group

LEGO® is a trademark of the LEGO Group, which does not sponsor, authorize or
endorse this project.
