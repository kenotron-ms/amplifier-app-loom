#!/usr/bin/env python3
"""
Prepare an Amplifier session and print its ID to stdout.

Usage:
    python3 prepare_session.py <project_path>

Output:
    A single session UUID on stdout. Nothing else.
    On error: message on stderr, exit code 1.

What this does:
    1. Detects the active bundle for the project (or defaults to "foundation").
    2. Calls load_and_prepare_bundle() — resolves all module paths from the
       amplifier cache. First run may download modules; subsequent runs are
       fast because everything is cached in ~/.amplifier/cache/.
    3. Saves a minimal session record to SessionStore so the CLI can resume it.
    4. Prints the session ID.

The caller (loom's spawnTerminal) stores this ID before spawning any PTY,
then always starts amplifier with:
    amplifier run --mode chat --resume <session_id>

This eliminates the fragile "read Session ID from the startup banner" pattern.
"""

import asyncio
import os
import sys
import uuid
from datetime import UTC, datetime
from pathlib import Path


def _detect_bundle(project_path: str) -> str:
    """
    Return the active bundle name for the given project directory.

    Resolution order (mirrors what `amplifier run` does):
      1. bundle.active from merged settings (user + project settings.yaml)
      2. bundle.md in <project>/.amplifier/  (project-level bundle)
      3. "foundation" as the global default
    """
    os.chdir(project_path)

    try:
        from amplifier_app_cli.lib.settings import AppSettings

        active = AppSettings().get_active_bundle()
        if active:
            return active
    except Exception:
        pass

    # Check for a project-level bundle.md
    local_bundle = Path(project_path) / ".amplifier" / "bundle.md"
    if local_bundle.exists():
        try:
            text = local_bundle.read_text(encoding="utf-8")
            # Extract name from YAML front-matter: "  name: loom"
            for line in text.splitlines():
                stripped = line.strip()
                if stripped.startswith("name:"):
                    name = stripped.split(":", 1)[1].strip()
                    if name:
                        return name
        except Exception:
            pass

    return "foundation"


async def _prepare(project_path: str) -> str:
    """
    Prepare the bundle, create a session record, and return the session ID.
    """
    os.chdir(project_path)

    bundle_name = _detect_bundle(project_path)
    session_id = str(uuid.uuid4())

    from amplifier_app_cli.lib.bundle_loader import AppBundleDiscovery
    from amplifier_app_cli.lib.bundle_loader.prepare import load_and_prepare_bundle
    from amplifier_app_cli.paths import get_bundle_search_paths
    from amplifier_app_cli.session_store import SessionStore

    discovery = AppBundleDiscovery(search_paths=get_bundle_search_paths())

    # Prepare the bundle — resolves/downloads all modules into ~/.amplifier/cache/.
    # install_deps=False skips pip installs (already done on first amplifier run).
    prepared = await load_and_prepare_bundle(
        bundle_name,
        discovery,
        install_deps=False,
    )

    # Save a minimal session record so `amplifier run --resume <id>` finds it.
    # The "bundle" key tells the CLI which bundle to load when resuming.
    store = SessionStore()
    metadata = {
        "session_id": session_id,
        "bundle": f"bundle:{bundle_name}",
        "working_dir": str(Path(project_path).resolve()),
        "created": datetime.now(UTC).isoformat(),
        "turn_count": 0,
        # Preserve module paths so the resume skips re-downloading modules.
        "bundle_context": {
            "module_paths": {k: str(v) for k, v in prepared.resolver._paths.items()},
            "mention_mappings": {},
        },
    }
    store.save(session_id, [], metadata)

    return session_id


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} <project_path>", file=sys.stderr)
        sys.exit(1)

    path = sys.argv[1]
    if not Path(path).is_dir():
        print(f"error: not a directory: {path!r}", file=sys.stderr)
        sys.exit(1)

    session_id = asyncio.run(_prepare(path))
    print(session_id)
