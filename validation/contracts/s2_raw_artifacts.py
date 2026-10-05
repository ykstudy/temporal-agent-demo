"""Independent raw ZIP, Go coverage, ELF/Mach-O format checks (standard library)."""
import io
import re
import stat
import struct
import zipfile

from .oracle import byte_hash, strict_loads
from .semantic_checks import require


def verify_raw(raw, expected):
    require(isinstance(raw,bytes) and len(raw)==expected["byte_count"] and byte_hash(raw)==expected["sha256"]==expected["ref"]["hash"],"raw artifact size/hash/reference mismatch")
    format_=expected["format"]
    if format_=="zip":
        manifest=expected["manifest"]
        files={f["path"]:f for f in manifest["files"]}
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            names=archive.namelist()
            require(len(names)==len(set(names))==len(files)+1,"ZIP duplicates or extra files")
            roots={name.split("/",1)[0] for name in names}
            require(len(roots)==1 and all(".." not in name.split("/") and not name.startswith("/") and "\\" not in name for name in names),"unsafe ZIP path")
            prefix=next(iter(roots))+"/"
            require(set(names)=={prefix+path for path in files}|{prefix+"AGENT-MANIFEST.json"},"ZIP differs from exact manifest file set")
            for info in archive.infolist(): require(not info.is_dir() and not stat.S_ISLNK(info.external_attr>>16),"ZIP non-file entry")
            for path,f in files.items():
                data=archive.read(prefix+path)
                require(len(data)==f["size"] and byte_hash(data)==f["sha256"],"ZIP file bytes differ from manifest")
            require(strict_loads(archive.read(prefix+"AGENT-MANIFEST.json"))==manifest,"ZIP manifest bytes describe different provenance")
    elif format_=="go_coverage":
        text=raw.decode("utf-8"); lines=text.splitlines()
        require(text.endswith("\n") and len(lines)>1 and lines[0] in ("mode: set","mode: count","mode: atomic"),"invalid complete coverage profile")
        for line in lines[1:]:
            m=re.fullmatch(r"(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)",line)
            require(m is not None,"malformed coverage record")
            start=(int(m[2]),int(m[3])); end=(int(m[4]),int(m[5])); statements=int(m[6]); count=int(m[7])
            require(all(x>0 for x in start+end) and end>=start and statements>0 and count<2**64 and (lines[0]!="mode: set" or count<=1),"invalid coverage counters/range")
    elif format_=="elf_executable":
        require(raw[:4]==b"\x7fELF" and len(raw)>=64 and raw[4]==2 and raw[5] in (1,2),"invalid ELF64 header")
        endian="<" if raw[5]==1 else ">"
        typ,machine=struct.unpack_from(endian+"HH",raw,16)
        entry,offset=struct.unpack_from(endian+"QQ",raw,24)
        size,count=struct.unpack_from(endian+"HH",raw,54)
        require(typ in (2,3) and machine in (62,183) and size>=56 and count>0 and offset+size*count<=len(raw),"invalid executable ELF structure")
        executable=False
        for i in range(count):
            kind,flags,fileoff,address,_,filesize,memsize,_=struct.unpack_from(endian+"IIQQQQQQ",raw,offset+i*size)
            require(fileoff+filesize<=len(raw),"ELF segment truncated")
            executable|=kind==1 and flags&1!=0 and address<=entry<address+memsize
        require(entry>0 and executable,"ELF entry point not executable")
    elif format_=="mach_o_executable":
        require(len(raw)>=32 and raw[:4] in (b"\xcf\xfa\xed\xfe",b"\xfe\xed\xfa\xcf"),"invalid Mach-O64 header")
        endian="<" if raw[:4]==b"\xcf\xfa\xed\xfe" else ">"
        _,cpu,_,typ,ncmds,cmdbytes,_,_=struct.unpack_from(endian+"8I",raw,0)
        require(cpu in (0x01000007,0x0100000c) and typ==2 and 32+cmdbytes<=len(raw),"invalid Mach-O executable structure")
        offset=32; found=False
        for _ in range(ncmds):
            require(offset+8<=32+cmdbytes,"Mach-O load command missing")
            command,size=struct.unpack_from(endian+"II",raw,offset)
            require(size>=8 and offset+size<=32+cmdbytes,"Mach-O command truncated")
            if command==0x19:
                require(size>=72,"Mach-O segment header truncated")
                _,_,segment,_,_,fileoff,filesize,_,_,sections,_=struct.unpack_from(endian+"II16sQQQQIIII",raw,offset)
                require(fileoff+filesize<=len(raw) and 72+sections*80<=size,"Mach-O segment/sections truncated")
                for i in range(sections):
                    section,_,_,length,position=struct.unpack_from(endian+"16s16sQQI",raw,offset+72+i*80)
                    if segment.rstrip(b"\0")==b"__TEXT" and section.rstrip(b"\0")==b"__text":
                        require(length>0 and position+length<=len(raw),"Mach-O text truncated")
                        found=True
            offset+=size
        require(found and offset==32+cmdbytes,"Mach-O executable text absent")
    else: raise ValueError("unsupported raw artifact format "+format_)
    return {"format":format_,"bytes":len(raw),"sha256":byte_hash(raw),"status":"passed"}
