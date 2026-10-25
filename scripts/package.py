"""Build a Linux x86-64 plugin archive with a static Go backend."""
import argparse
import os
from pathlib import Path
import subprocess
import struct
import tarfile
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('output', type=Path, help='New .tar.gz archive')
    parser.add_argument('--backend', type=Path, help='Use a prebuilt static Linux x86-64 backend')
    parser.add_argument('--go', default=os.environ.get('GO', 'go'), help='Go compiler executable (build machines only)')
    args = parser.parse_args()
    here = Path(__file__).resolve().parents[1]
    files = [(here/'rnl-virtuoso.il', 'rnl-virtuoso.il'), (here/'rnl-library.il', 'rnl-library.il'),
             (here/'rnl-api.il', 'rnl-api.il'), (here/'README.md', 'README.md'),
             (here/'LICENSE', 'LICENSE'), (here/'Go-LICENSE.txt', 'Go-LICENSE.txt')]
    files.extend((p, p.relative_to(here).as_posix()) for p in (here/'docs').glob('*.md'))
    files.extend((p, p.relative_to(here).as_posix()) for p in (here/'resources').iterdir() if p.is_file())
    files.extend((here/'LICENSES'/name, 'LICENSES/'+name) for name in ('MIT.txt', 'CC0-1.0.txt', 'BSD-3-Clause.txt'))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='rnl-go-build-') as temporary:
        backend = args.backend
        if backend is None:
            backend = Path(temporary)/'rnl-virtuoso'
            env = dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH='amd64', GOAMD64='v1', GOTOOLCHAIN='local')
            subprocess.run([args.go, 'build', '-buildvcs=false', '-trimpath', '-ldflags=-s -w', '-o', str(backend), './cmd/rnl-virtuoso'],
                           cwd=here, env=env, check=True)
        binary = backend.read_bytes()
        header = binary[:64]
        if len(header) != 64 or header[:6] != b'\x7fELF\x02\x01' or header[18:20] != b'\x3e\x00':
            raise ValueError('Backend must be a Linux x86-64 ELF executable')
        phoff, = struct.unpack_from('<Q', header, 32)
        phsize, phnum = struct.unpack_from('<HH', header, 54)
        if phsize < 56 or phoff + phsize * phnum > len(binary):
            raise ValueError('Invalid ELF program header table')
        if any(struct.unpack_from('<I', binary, phoff+i*phsize)[0] in (2, 3) for i in range(phnum)):
            raise ValueError('Backend must be statically linked without an ELF interpreter or dynamic dependencies')
        files.append((backend, 'bin/rnl-virtuoso'))
        with args.output.open('xb') as stream, tarfile.open(fileobj=stream, mode='w:gz') as archive:
            for path, name in sorted(files, key=lambda pair: pair[1]):
                info = archive.gettarinfo(str(path), arcname='rnl-virtuoso/'+name)
                info.mode = 0o755 if name == 'bin/rnl-virtuoso' else 0o644
                with path.open('rb') as source:
                    archive.addfile(info, source)
    print(args.output.resolve())


if __name__ == '__main__':
    main()
