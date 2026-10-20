"""Build and run the shared Python/Go regression suite from any working directory."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', default=os.environ.get('GO', 'go'), help='Go compiler executable')
    parser.add_argument('--rnl', default=os.environ.get('RNL_BIN') or shutil.which('rnl'), help='Optional RNL checker executable')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    target = root/'target'
    target.mkdir(exist_ok=True)
    env = dict(os.environ, GOTOOLCHAIN='local', CGO_ENABLED='0')
    for key in ('GOOS', 'GOARCH', 'GOAMD64'):
        env.pop(key, None)

    def run(command, environment=env):
        subprocess.run(command, cwd=root, env=environment, check=True)

    run([args.go, 'test', './...'])
    run([args.go, 'vet', './...'])
    host = target/('rnl-virtuoso.exe' if os.name == 'nt' else 'rnl-virtuoso')
    linux = target/'rnl-virtuoso-linux-amd64'
    run([args.go, 'build', '-buildvcs=false', '-o', str(host), './cmd/rnl-virtuoso'])
    run([args.go, 'build', '-buildvcs=false', '-trimpath', '-ldflags=-s -w', '-o', str(linux), './cmd/rnl-virtuoso'],
        dict(env, GOOS='linux', GOARCH='amd64', GOAMD64='v1'))
    if args.rnl:
        env['RNL_BIN'] = str(Path(args.rnl).resolve())
    else:
        print('RNL checker not found: drawing/checker assertions will be skipped.', flush=True)
    env['RNL_EXPORT_BIN'] = str(host)
    env['RNL_EXPORT_LINUX_BIN'] = str(linux)
    for backend in ('python', 'go'):
        print('Fixture backend: '+backend, flush=True)
        run([sys.executable, '-m', 'unittest', 'discover', '-s', 'tests', '-v'],
            dict(env, RNL_EXPORT_BACKEND=backend))


if __name__ == '__main__':
    main()
