"""Private machine state for executable smoke tests, including their children."""
from contextlib import contextmanager
import json
import os
from pathlib import Path
import tempfile


@contextmanager
def isolated_atlas_environment():
    with tempfile.TemporaryDirectory(prefix="atlas-smoke-home-") as temporary:
        home = Path(temporary)
        state = home / "state" / "atlas-tasker"
        state.mkdir(parents=True, mode=0o700)
        settings = state / "settings.json"
        settings.write_text(json.dumps({
            "format": "atlas_machine_settings_v1",
            "service": {"enabled": False, "auto_start": False},
            "agents": {"auto_install": False},
            "browser": {"open_home": False},
        }), encoding="utf-8")
        settings.chmod(0o600)
        values = {"HOME": str(home), "XDG_STATE_HOME": str(home / "state"),
                  "XDG_CONFIG_HOME": str(home / "config")}
        previous = {key: os.environ.get(key) for key in values}
        os.environ.update(values)
        try:
            yield
        finally:
            for key, value in previous.items():
                if value is None:
                    os.environ.pop(key, None)
                else:
                    os.environ[key] = value
