# Provenance and licenses

This repository does **not** contain the parts libraries, and does not contain
the files generated from them. Both are fetched or built on first use. That is
deliberate: the licenses permit use and redistribution, but they attach
conditions to anything derived, and you do not want those pulled into your own
repository unnoticed.

## LDraw Parts Library

**CCAL 2.0** — the Creative Commons Attribution License 2.0, as set out in the
library's own `CAreadme.txt` and `CAlicense.txt`. Every part file says so in its
header:

```text
0 !LICENSE Redistributable under CCAL version 2.0 : see CAreadme.txt
```

Fetched whole from <https://library.ldraw.org/library/updates/complete.zip> and
extracted into `~/.cache/brickmesh-ldraw/ldraw`, licence files included — the
condition is that anyone you pass the data to can find out the terms, and
shipping the parts while leaving the licence behind would not meet that.

The library is the work of LDraw.org volunteers, and each `.dat` names its own
author. The LDraw Steering Committee holds an attribution to **The LDraw Parts
Library** to be sufficient in a derivative work in lieu of a full author list,
which is the form used here.

LDraw is a trademark of the LDraw.org organisation.

> Previously this was fetched a file at a time from a third-party mirror, which
> turned out to be a snapshot predating every part released since about 2015 —
> 17% of the Technic parts were simply absent. See `docs/findings.md`.

## LDCad Shadow Library

Roland Melkert, under **CC BY-SA 4.0**. This is the layer describing, per part,
where the holes and pins sit and which way they point.

Mind the share-alike condition: a file derived from this data falls under the
same license. The code that processes it does not.

## What that means for the generated files

`make assets` writes two files, and they do not carry the same terms, because
they are not derived from the same thing.

| file | derived from | terms |
| --- | --- | --- |
| `catalog.bin` | shadow library ports, LDraw ids and titles | **CC BY-SA 4.0** |
| `meshes.bin` | LDraw part geometry | **CC BY 2.0** (CCAL 2.0) |

`catalog.bin` is share-alike because the shadow library is, and that is the
stricter of the two.

`meshes.bin` is a derivative work of the parts library and not one of the
exceptions its licence lists: it is neither a rendered image nor a model file
containing only references — it is the part geometry itself, converted to
another format. So it carries attribution and the licence notice.

The generator writes a `LICENSE.txt` beside them saying exactly this. If you
publish the files, publish that too, and put the notice next to the download
link rather than only in a repository somewhere.

## This code

Apache License 2.0, see `LICENSE` and `NOTICE`. The engine is not derived from
either library; it reads them.

`web/wasm_exec.js` is from the Go distribution, © The Go Authors, BSD-3-Clause.

## No affiliation with the LEGO Group

LEGO is a trademark of the LEGO Group, which does not sponsor, authorize or
endorse this project.
