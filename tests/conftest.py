# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Test wiring for the fixture parts.

The extractor normally fetches the LDraw and LDCad libraries on first use and
caches them under ~/.cache. Both lookups check the cache directory first, so
pointing that directory at tests/fixtures is enough to run the real code paths
against synthetic parts, with no download and no dependency on what happens to
be cached on the machine.

See tests/fixtures/generate_fixtures.py for what those parts are and why they
are not real ones.
"""
from __future__ import annotations

import urllib.request
from pathlib import Path

import pytest
from brickmesh_extract import catalog, ldraw, snap, voxel

FIXTURES = Path(__file__).resolve().parent / "fixtures"
LDRAW_FIXTURES = FIXTURES / "ldraw"
SHADOW_FIXTURES = FIXTURES / "shadow"


@pytest.fixture(autouse=True)
def no_network(monkeypatch):
    """
    Nothing in the suite may reach the network. Without this a missing fixture
    would quietly fall through to a download: slow, flaky offline, and the
    failure would point at the wrong thing.
    """
    def forbidden(url, *a, **kw):
        raise AssertionError(
            f"a test tried to download {url}. Fixtures live in {FIXTURES}; "
            "add the part there rather than fetching it.")

    monkeypatch.setattr(urllib.request, "urlopen", forbidden)
    monkeypatch.setattr(urllib.request, "urlretrieve", forbidden)


@pytest.fixture
def parts(monkeypatch):
    """The LDraw side: ldraw.geometry() resolves against the fixture parts."""
    monkeypatch.setattr(ldraw, "CACHE", str(LDRAW_FIXTURES))
    # These caches key on part name only, so they would leak between tests and
    # across the real cache directory.
    monkeypatch.setattr(ldraw, "_geo_cache", {})
    monkeypatch.setattr(voxel, "_vox_cache", {})
    return LDRAW_FIXTURES


@pytest.fixture
def shadow(monkeypatch):
    """
    The LDCad side. ensure_library() returns early when the extracted directory
    is already there, so a fixture tree named the same way is picked up as if
    it had just been downloaded.
    """
    monkeypatch.setattr(snap, "SHADOW_DIR", str(SHADOW_FIXTURES))
    monkeypatch.setattr(catalog, "_pcache", {}, raising=False)
    return SHADOW_FIXTURES / "LDCadShadowLibrary-main"
