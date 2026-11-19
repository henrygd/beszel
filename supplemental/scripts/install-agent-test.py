#!/usr/bin/env python3
"""Exercise the installer's systemd configuration in a disposable directory.

Run with python3 supplemental/scripts/install-agent-test.py [installer-path].
Only chown is stubbed: native root ownership and service-manager access require
the privileged installation check. No system service or account is modified.
"""

import os
from pathlib import Path
import subprocess
import sys
import tempfile


source = Path(sys.argv[1] if len(sys.argv) > 1 else Path(__file__).with_name("install-agent.sh")).read_text()
helpers = source.split("# Expected failures must be handled explicitly;")[0]
setup = source.split("  # Original systemd service installation code\n", 1)[1].split("  # Load and start the service\n", 1)[0]


def install(root, token="", provided=False, mask="022", expected=0):
    unit = root / "beszel-agent.service"
    script = helpers + """
set -eu
fail() { echo "$*" >&2; exit 1; }
chown() { :; }
detect_nvidia_devices() { :; }
PORT=45876 KEY=public-key HUB_URL=https://hub.example
PORT_PROVIDED=false KEY_PROVIDED=false HUB_URL_PROVIDED=false
BIN_PATH=/bin/true
umask "$TEST_UMASK"
if command -v read_systemd_token >/dev/null; then
  read_systemd_token "$TEST_UNIT"
fi
""" + setup.replace("/etc/systemd/system/beszel-agent.service", str(unit))
    env = dict(os.environ, AGENT_DIR=str(root), TEST_UNIT=str(unit), TOKEN=token,
               TOKEN_PROVIDED=str(provided).lower(), TEST_UMASK=mask)
    result = subprocess.run(["sh"], input=script, text=True, env=env, capture_output=True)
    assert result.returncode == expected, result.stderr
    return unit


with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    for mask in ("000", "022", "077"):
        case = root / mask
        case.mkdir(mode=0o755)
        unit = install(case, "sample-credential", True, mask)
        secret = case / "token.env"
        assert "sample-credential" not in unit.read_text(), "token remains in public unit"
        assert unit.stat().st_mode & 0o022 == 0, "public unit is writable by other users"
        assert secret.read_text() == 'TOKEN="sample-credential"\n'
        assert secret.stat().st_mode & 0o777 == 0o600
        assert f'EnvironmentFile={secret}\n' in unit.read_text()

        # Omitted arguments preserve both the token and unrelated custom settings.
        unit.write_text(unit.read_text() + "\n# custom setting\n")
        original = secret.read_bytes()
        install(case)
        assert secret.read_bytes() == original
        assert "# custom setting" in unit.read_text()
        install(case, "rotated-credential", True)
        assert secret.read_text() == 'TOKEN="rotated-credential"\n'
        assert unit.read_text().count("EnvironmentFile=") == 1
        assert not list(case.glob(".token.env.*"))

    legacy = root / "legacy"
    legacy.mkdir()
    unit = legacy / "beszel-agent.service"
    unit.write_text('[Service]\nEnvironment="TOKEN=#custom-token"\nEnvironment="KEY=existing-key TOKEN=comment"\n'
                    f'# EnvironmentFile="{legacy / "token.env"}"\n')
    unit.chmod(0o666)
    install(legacy)
    assert unit.stat().st_mode & 0o022 == 0
    assert (legacy / "token.env").read_text() == 'TOKEN="#custom-token"\n'
    assert 'Environment="TOKEN=' not in unit.read_text()
    assert 'Environment="KEY=existing-key TOKEN=comment"' in unit.read_text()
    assert f'\nEnvironmentFile={legacy / "token.env"}\n' in unit.read_text()

    # Custom file-based credentials and old SSH-only units have no inline token.
    for config in ('Environment="TOKEN_FILE=/private/custom-token"', 'Environment="KEY=ssh-only"'):
        unit.write_text(f"[Service]\n{config}\n")
        before = unit.read_bytes()
        install(legacy)
        assert unit.read_bytes() == before

    # Reject ambiguous/manual encodings before any credential or unit mutation.
    for config in ('Environment="TOKEN=one"\nEnvironment="TOKEN=two"',
                   'Environment="KEY=key" "TOKEN=custom"',
                   'Environment=KEY=key TOKEN=custom',
                   'Environment="BESZEL_AGENT_TOKEN=custom"',
                   'Environment="TOKEN=percent%%value"',
                   r'Environment="TOKEN=escaped\x2dvalue"'):
        unit.write_text(f"[Service]\n{config}\n")
        before = unit.read_bytes(), (legacy / "token.env").read_bytes()
        install(legacy, expected=1)
        assert before == (unit.read_bytes(), (legacy / "token.env").read_bytes())

    # EnvironmentFile quoting preserves custom token syntax without evaluation.
    unit.write_text('[Service]\nEnvironment="TOKEN=old"\n')
    install(legacy, ' #custom $value `command` "quote" \\ ', True)
    assert (legacy / "token.env").read_text() == 'TOKEN=" #custom \\$value \\`command\\` \\"quote\\" \\\\ "\n'
    unit.write_text('[Service]\nEnvironment="TOKEN=percent%%value"\n')
    install(legacy, "percent%value", True)
    assert (legacy / "token.env").read_text() == 'TOKEN="percent%value"\n'

    ssh_only = root / "ssh-only"
    ssh_only.mkdir()
    install(ssh_only)
    assert (ssh_only / "token.env").read_text() == 'TOKEN=""\n'

print("PASS: systemd generation, migration, rotation, optional tokens, permissions and failure controls")
