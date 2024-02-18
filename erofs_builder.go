package styx

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"path"
	"sort"

	"github.com/lunixbochs/struc"
	"github.com/nix-community/go-nix/pkg/nar"
	"golang.org/x/sys/unix"
)

type (
	builder struct {
		err error
		nar *nar.Reader
		out io.Writer

		blk blkshift
	}

	regulardata struct {
		inum uint32
		data []byte
	}

	inodebuilder struct {
		i        erofs_inode_compact
		taildata []byte
		nid      uint64
	}

	dbent struct {
		name string
		inum uint32
		tp   uint16
	}

	dirbuilder struct {
		ents []dbent
		inum uint32
		size int64
	}
)

func NewBuilder(r io.Reader, out io.Writer) *builder {
	nr, err := nar.NewReader(r)
	if err != nil {
		return &builder{err: err}
	}
	return &builder{nar: nr, out: out, blk: blkshift(12)}
}

func (b *builder) Build() error {
	if b.err != nil {
		return b.err
	}

	const inodeshift = blkshift(5)
	const inodesize = 1 << inodeshift

	var root uint32
	var datablocks []regulardata
	var inodes []*inodebuilder
	dirsmap := make(map[string]*dirbuilder)
	var dirs []*dirbuilder

	// pass 1: read nar

	for {
		h, err := b.nar.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		} else if err = h.Validate(); err != nil {
			return err
		} else if h.Path == "/" && h.Type != nar.TypeDirectory {
			return errors.New("can't handle bare file nars yet")
		}

		// every header gets an inode and all but the root get a dirent
		var fstype uint16
		inum := uint32(len(inodes))
		i := &inodebuilder{
			i: erofs_inode_compact{
				IFormat: ((EROFS_INODE_LAYOUT_COMPACT << EROFS_I_VERSION_BIT) |
					(EROFS_INODE_FLAT_INLINE << EROFS_I_DATALAYOUT_BIT)),
				IIno:   inum + 37,
				INlink: 1,
			},
		}
		inodes = append(inodes, i)

		switch h.Type {
		case nar.TypeDirectory:
			fstype = EROFS_FT_DIR
			i.i.IMode = unix.S_IFDIR | 0755
			i.i.INlink = 2
			parent := inum
			if h.Path != "/" {
				parent = dirsmap[path.Dir(h.Path)].inum
			}
			db := &dirbuilder{
				ents: []dbent{
					{name: ".", tp: EROFS_FT_DIR, inum: inum},
					{name: "..", tp: EROFS_FT_DIR, inum: parent},
				},
				inum: inum,
			}
			dirs = append(dirs, db)
			dirsmap[h.Path] = db

		case nar.TypeRegular:
			fstype = EROFS_FT_REG_FILE
			i.i.IMode = unix.S_IFREG | 0644
			if h.Executable {
				i.i.IMode = unix.S_IFREG | 0755
			}
			if h.Size > 0 {
				if h.Size > math.MaxUint32 {
					return fmt.Errorf("TODO: support larger files with extended inode")
				}
				i.i.ISize = uint32(h.Size)
				data := make([]byte, h.Size)
				n, err := b.nar.Read(data)
				if err != nil || n < int(h.Size) {
					return fmt.Errorf("short read on %#v", h)
				}

				tail := b.blk.leftover(h.Size)
				if tail == 0 || tail > b.blk.size()-inodesize {
					// tail has to fit in a block with the inode
					tail = 0
					i.i.IFormat = ((EROFS_INODE_LAYOUT_COMPACT << EROFS_I_VERSION_BIT) |
						(EROFS_INODE_FLAT_PLAIN << EROFS_I_DATALAYOUT_BIT))
				}
				full := h.Size - tail
				if full > 0 {
					datablocks = append(datablocks, regulardata{
						inum: inum,
						data: data[:full],
					})
				}
				i.taildata = data[full:]
			}

		case nar.TypeSymlink:
			fstype = EROFS_FT_SYMLINK
			i.i.IMode = unix.S_IFLNK | 0777
			i.i.ISize = uint32(len(h.LinkTarget))
			i.taildata = []byte(h.LinkTarget)

		default:
			return errors.New("unknown type")
		}

		// add dirent to appropriate directory
		if h.Path == "/" {
			root = inum
		} else {
			dir, file := path.Split(h.Path)
			db := dirsmap[path.Clean(dir)]
			db.ents = append(db.ents, dbent{
				name: file,
				inum: inum,
				tp:   fstype,
			})
		}
	}

	// pass 2: pack inodes and tails

	// 2.1: directory sizes
	dummy := make([]byte, b.blk.size())
	for _, db := range dirs {
		db.sortAndSize(b.blk)
		tail := b.blk.leftover(db.size)
		// dummy data just to track size
		inodes[db.inum].taildata = dummy[:tail]
	}

	// at this point:
	// all inodes have correct len(taildata)
	// files have correct taildata but dirs do not

	inodebase := max(4096, b.blk.size())

	// 2.2: lay out inodes and tails
	// using greedy for now. TODO: use best fit or something
	p := int64(0) // position relative to metadata area
	remaining := b.blk.size()
	for i, inode := range inodes {
		need := inodesize + inodeshift.roundup(int64(len(inode.taildata)))
		if need > remaining {
			p += remaining
			remaining = b.blk.size()
		}
		inodes[i].nid = uint64(p >> inodeshift)
		p += need
		remaining -= need
	}
	if remaining < b.blk.size() {
		p += remaining
	}

	// at this point:
	// all inodes have correct nid

	// 2.3: write actual tails of dirs to tails
	inumToNid := func(inum uint32) uint64 { return inodes[inum].nid }
	for _, db := range dirs {
		var buf bytes.Buffer
		db.write(&buf, true, b.blk, inumToNid)
		if len(inodes[db.inum].taildata) != buf.Len() {
			panic("oops, bug")
		}
		inodes[db.inum].taildata = buf.Bytes()
	}

	// at this point:
	// all inodes have correct taildata

	// from here on, p is relative to 0
	p += inodebase

	// 2.4: lay out dirs
	for _, db := range dirs {
		inodes[db.inum].i.IU = uint32(p >> b.blk)
		inodes[db.inum].i.ISize = uint32(db.size)
		tail := b.blk.leftover(db.size)
		if tail > b.blk.size()-inodesize {
			panic("fix this!")
		}
		full := db.size - tail
		p += full
	}
	// 2.5: lay out rest of blocks
	for _, data := range datablocks {
		inodes[data.inum].i.IU = uint32(p >> b.blk)
		p += b.blk.roundup(int64(len(data.data)))
	}
	fmt.Printf("final calculated size %d\n", p)

	// at this point:
	// all inodes have correct IU and ISize

	// pass 3: write

	// 3.1: fill in super
	super := erofs_super_block{
		Magic:       0xE0F5E1E2,
		BlkSzBits:   uint8(b.blk),
		RootNid:     uint16(root),
		Inos:        uint64(len(inodes)),
		Blocks:      uint32(p >> b.blk),
		MetaBlkAddr: uint32(inodebase >> b.blk),
	}
	// TODO: make uuid from nar hash to be reproducible
	rand.Read(super.Uuid[:])
	// TODO: append a few bits of nar hash to volname
	copy(super.VolumeName[:], "styx-test")

	c := crc32.NewIEEE()
	pack(c, &super)
	super.Checksum = c.Sum32()

	pad(b.out, 1024)
	pack(b.out, &super)
	pad(b.out, inodebase-1024-128)

	// 3.2: inodes and tails
	remaining = b.blk.size()
	for _, i := range inodes {
		need := inodesize + inodeshift.roundup(int64(len(i.taildata)))
		if need > remaining {
			pad(b.out, remaining)
			remaining = b.blk.size()
		}
		pack(b.out, i.i)
		writeAndPad(b.out, i.taildata, inodeshift)
		remaining -= need
	}
	if remaining < b.blk.size() {
		pad(b.out, remaining)
	}

	// 3.3: dirs
	for _, db := range dirs {
		db.write(b.out, false, b.blk, inumToNid)
	}
	// 3.4: data
	for _, data := range datablocks {
		writeAndPad(b.out, data.data, b.blk)
	}

	return nil
}

func (db *dirbuilder) sortAndSize(shift blkshift) {
	const direntsize = 12

	sort.Slice(db.ents, func(i, j int) bool { return db.ents[i].name < db.ents[j].name })

	blocks := int64(0)
	remaining := shift.size()

	for _, ent := range db.ents {
		need := int64(direntsize + len(ent.name))
		if need > remaining {
			blocks++
			remaining = shift.size()
		}
		remaining -= need
	}

	db.size = blocks<<shift + (shift.size() - remaining)
}

func (db *dirbuilder) write(out io.Writer, writeTail bool, shift blkshift, inumToNid func(uint32) uint64) {
	const direntsize = 12

	remaining := shift.size()
	ents := make([]erofs_dirent, 0, shift.size()/16)
	var names bytes.Buffer

	flush := func(isTail bool) {
		if writeTail == isTail {
			nameoff0 := uint16(len(ents) * direntsize)
			for i := range ents {
				ents[i].NameOff += nameoff0
			}
			pack(out, ents)
			io.Copy(out, &names)
			if !isTail {
				pad(out, remaining)
			}
		}

		ents = ents[:0]
		names.Reset()
		remaining = shift.size()
	}

	for _, ent := range db.ents {
		need := int64(direntsize + len(ent.name))
		if need > remaining {
			flush(false)
		}

		ents = append(ents, erofs_dirent{
			Nid:      inumToNid(ent.inum),
			NameOff:  uint16(names.Len()), // offset minus nameoff0
			FileType: uint8(ent.tp),
		})
		names.Write([]byte(ent.name))
		remaining -= need
	}
	flush(true)

	return
}

var _zeros = make([]byte, 4096)

func pad(out io.Writer, n int64) error {
	for {
		if n <= 4096 {
			_, err := out.Write(_zeros[:n])
			return err
		}
		out.Write(_zeros)
		n -= 4096
	}
}

var _popts = struc.Options{Order: binary.LittleEndian}

func pack(out io.Writer, v any) error {
	return struc.PackWithOptions(out, v, &_popts)
}

func writeAndPad(out io.Writer, data []byte, shift blkshift) error {
	n, err := out.Write(data)
	r := shift.roundup(int64(n)) - int64(n)
	pad(out, r)
	return err
}
