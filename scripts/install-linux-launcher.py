import argparse
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile


def desktop_value(value):
    return str(value).replace("\\", "\\\\").replace("\n", "\\n").replace("\r", "\\r").replace("\t", "\\t")


def desktop_command(path):
    value = str(path).replace("%", "%%")
    for character in ("\\", '"', "`", "$"):
        value = value.replace(character, "\\" + character)
    return desktop_value('"' + value + '"')


def write_atomic(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    staged = None
    try:
        with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as output:
            staged = Path(output.name)
            output.write(content)
        staged.chmod(0o644)
        os.replace(staged, path)
    finally:
        if staged is not None and staged.exists():
            staged.unlink()


def main():
    parser = argparse.ArgumentParser(description="Add Praewa Code to the Linux application menu.")
    parser.add_argument("--data-dir", type=Path, help="Desktop data directory; defaults to the current user's application data.")
    args = parser.parse_args()
    if platform.system() != "Linux":
        parser.error("This launcher is for Linux desktops.")
    root = Path(__file__).resolve().parents[1]
    binary = root / "build" / "bin" / "praewa-code"
    if not binary.is_file() or not os.access(binary, os.X_OK):
        parser.error("Build Praewa Code first with make build.")
    data_dir = (args.data_dir or Path(os.environ.get("XDG_DATA_HOME") or Path.home() / ".local" / "share")).expanduser().resolve()
    applications = data_dir / "applications"
    icon = data_dir / "icons" / "hicolor" / "scalable" / "apps" / "praewa-code.svg"
    launcher = applications / "praewa-code.desktop"
    content = "\n".join([
        "[Desktop Entry]",
        "Type=Application",
        "Name=Praewa Code",
        "GenericName=Code Editor",
        "GenericName[th]=โปรแกรมเขียนโค้ด",
        "Comment=Code editor with an integrated terminal and agents",
        "Comment[th]=โปรแกรมเขียนโค้ดพร้อม Terminal และ Agent",
        "Keywords=Praewa;Praewa Code;แพรวา;โค้ด;Code;Editor;IDE;Agent;Compile;Terminal;",
        f"Exec={desktop_command(binary)}",
        f"TryExec={desktop_value(binary)}",
        f"Path={desktop_value(root)}",
        f"Icon={desktop_value(icon)}",
        "Terminal=false",
        "StartupNotify=true",
        "Categories=Development;IDE;",
        "",
    ])
    validator = shutil.which("desktop-file-validate")
    if validator:
        with tempfile.TemporaryDirectory(prefix="praewa-launcher-") as preview:
            candidate = Path(preview) / launcher.name
            candidate.write_text(content, encoding="utf-8")
            subprocess.run([validator, str(candidate)], check=True)
    write_atomic(icon, (root / "frontend" / "public" / "favicon.svg").read_bytes())
    write_atomic(launcher, content.encode("utf-8"))
    updater = shutil.which("update-desktop-database")
    if updater:
        subprocess.run([updater, str(applications)], check=True)
    print(f"Installed: {launcher}")
    print("Search for Praewa Code or แพรวา in your application menu.")


if __name__ == "__main__":
    main()
