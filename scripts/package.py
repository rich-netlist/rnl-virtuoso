"""Build a Linux x86-64 plugin archive with a static Go backend, and beside it the agent skill
as rnl-virtuoso-skill.zip, each with a .sha256 file."""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess
import struct
import tarfile
import tempfile
import time
import zipfile

SKILL_ZIP = 'rnl-virtuoso-skill.zip'
# The MIT license of the code, docs and skill, as the plugin archive and the skill zip carry it.
MIT_FILES = ('LICENSE', 'LICENSES/MIT.txt')


def write_sha256(path):
    """Write `<path>.sha256` in the format of sha256sum, naming the file without its directory."""
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    with path.with_name(path.name + '.sha256').open('x', encoding='utf-8', newline='\n') as stream:
        stream.write(f'{digest}  {path.name}\n')


def write_skill_zip(here, output):
    """The agent skill under one folder `rnl-virtuoso/`, with the MIT license. Every entry has the
    same time, SOURCE_DATE_EPOCH if set and 1980-01-01 otherwise, and Unix mode 0644, so the same
    sources give the same bytes on any machine."""
    epoch = os.environ.get('SOURCE_DATE_EPOCH')
    stamp = time.gmtime(int(epoch))[:6] if epoch else (1980, 1, 1, 0, 0, 0)
    skill = here/'skills'/'rnl-virtuoso'
    files = [(p, p.relative_to(skill).as_posix()) for p in skill.rglob('*') if p.is_file()]
    files.extend((here/name, name) for name in MIT_FILES)
    with output.open('xb') as stream, zipfile.ZipFile(stream, 'w') as archive:
        for path, name in sorted(files, key=lambda pair: pair[1]):
            info = zipfile.ZipInfo('rnl-virtuoso/'+name, date_time=stamp)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3
            info.external_attr = 0o100644 << 16
            archive.writestr(info, path.read_bytes())
    write_sha256(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('output', type=Path, help='New .tar.gz archive')
    parser.add_argument('--backend', type=Path, help='Use a prebuilt static Linux x86-64 backend')
    parser.add_argument('--go', default=os.environ.get('GO', 'go'), help='Go compiler executable (build machines only)')
    args = parser.parse_args()
    here = Path(__file__).resolve().parents[1]
    files = [(here/'rnl-virtuoso.il', 'rnl-virtuoso.il'), (here/'rnl-library.il', 'rnl-library.il'),
             (here/'rnl-api.il', 'rnl-api.il'), (here/'README.md', 'README.md'),
             (here/'Go-LICENSE.txt', 'Go-LICENSE.txt')]
    files.extend((here/name, name) for name in MIT_FILES)
    files.extend((p, p.relative_to(here).as_posix()) for p in (here/'docs').glob('*.md'))
    files.extend((p, p.relative_to(here).as_posix()) for p in (here/'resources').iterdir() if p.is_file())
    files.extend((here/'LICENSES'/name, 'LICENSES/'+name) for name in ('CC0-1.0.txt', 'BSD-3-Clause.txt'))
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
    write_sha256(args.output)
    skill_zip = args.output.with_name(SKILL_ZIP)
    write_skill_zip(here, skill_zip)
    print(args.output.resolve())
    print(skill_zip.resolve())


if __name__ == '__main__':
    main()
