package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/lunixbochs/struc"
	"github.com/nix-community/go-nix/pkg/narinfo/signature"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	"github.com/dnr/styx/common"
	"github.com/dnr/styx/erofs"
	"github.com/dnr/styx/manifester"
	"github.com/dnr/styx/pb"
)

const (
	typeImage uint16 = iota
	typeSlab
)

type (
	server struct {
		cfg         *Config
		digestBytes int
		blockShift  blkshift
		csread      manifester.ChunkStoreRead
		mcread      manifester.ChunkStoreRead
		db          *bbolt.DB
		pool        *sync.Pool
		sf          singleflight.Group
		builder     *erofs.Builder
		devnode     int
		zeros       []byte

		lock       sync.Mutex
		cacheState map[uint32]*openFileState // object id -> state
	}

	openFileState struct {
		fd uint32
		tp uint16

		// for slabs
		slabId uint16

		// for images
		imageData       []byte
		hasWrittenImage bool
	}

	Config struct {
		DevPath   string
		CachePath string

		StyxPubKeys []signature.PublicKey
		Params      pb.DaemonParams

		Upstream string

		ErofsBlockShift int
		SmallFileCutoff int

		Workers int
	}
)

var _ erofs.SlabManager = (*server)(nil)

func CachefilesServer(cfg Config) *server {
	return &server{
		cfg:         &cfg,
		digestBytes: int(cfg.Params.Params.DigestBits >> 3),
		blockShift:  blkshift(cfg.ErofsBlockShift),
		csread:      manifester.NewChunkStoreReadUrl(cfg.Params.ChunkReadUrl, manifester.ChunkReadPath),
		mcread:      manifester.NewChunkStoreReadUrl(cfg.Params.ManifestCacheUrl, manifester.ManifestCachePath),
		pool:        &sync.Pool{New: func() any { return make([]byte, CACHEFILES_MSG_MAX_SIZE) }},
		builder:     erofs.NewBuilder(erofs.BuilderConfig{BlockShift: cfg.ErofsBlockShift}),
		cacheState:  make(map[uint32]*openFileState),
		zeros:       make([]byte, 1<<cfg.ErofsBlockShift),
	}
}

func (s *server) openDb() (err error) {
	opts := bbolt.Options{
		NoFreelistSync: true,
		FreelistType:   bbolt.FreelistMapType,
	}
	s.db, err = bbolt.Open(path.Join(s.cfg.CachePath, dbFilename), 0644, &opts)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		var gp pb.GlobalParams
		if mb, err := tx.CreateBucketIfNotExists(metaBucket); err != nil {
			return err
		} else if _, err = tx.CreateBucketIfNotExists(chunkBucket); err != nil {
			return err
		} else if _, err = tx.CreateBucketIfNotExists(slabBucket); err != nil {
			return err
		} else if _, err = tx.CreateBucketIfNotExists(imageBucket); err != nil {
			return err
		} else if b := mb.Get(metaParams); b == nil {
			if b, err = proto.Marshal(s.cfg.Params.Params); err != nil {
				return err
			}
			mb.Put(metaParams, b)
		} else if err = proto.Unmarshal(b, &gp); err != nil {
			return err
		} else if mp := s.cfg.Params.Params; false ||
			gp.ChunkShift != mp.ChunkShift ||
			gp.DigestAlgo != mp.DigestAlgo ||
			gp.DigestBits != mp.DigestBits {
			return fmt.Errorf("mismatched global params; wipe cache and start over")
		}
		return nil
	})
}

func (s *server) startSocketServer() (err error) {
	socketPath := path.Join(s.cfg.CachePath, Socket)
	os.Remove(socketPath)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: socketPath})
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mount", s.handleMountReq)
	mux.HandleFunc("/umount", s.handleUmountReq)
	mux.HandleFunc("/delete", s.handleDeleteReq)
	go http.Serve(l, mux)
	return nil
}

func (s *server) handleMountReq(w http.ResponseWriter, req *http.Request) {
	var r MountReq
	var res MountResp
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	// TODO

	json.NewEncoder(w).Encode(res)
}

func (s *server) handleUmountReq(w http.ResponseWriter, req *http.Request) {
	var r UmountReq
	var res UmountResp
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	// TODO

	json.NewEncoder(w).Encode(res)
}

func (s *server) handleDeleteReq(w http.ResponseWriter, req *http.Request) {
	var r DeleteReq
	var res DeleteResp
	if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	// TODO

	json.NewEncoder(w).Encode(res)
}

func (s *server) setupEnv() error {
	err := exec.Command("modprobe", "cachefiles").Run()
	if err != nil {
		return err
	}
	return os.MkdirAll(s.cfg.CachePath, 0700)
}

func (s *server) openDevNode() (err error) {
	// TODO: support systemd fd saving so we can "restore" inflight requests after restart

	s.devnode, err = unix.Open(s.cfg.DevPath, unix.O_RDWR, 0600)
	if err == unix.ENOENT {
		_ = unix.Mknod(s.cfg.DevPath, 0600|unix.S_IFCHR, 10<<8+122)
		s.devnode, err = unix.Open(s.cfg.DevPath, unix.O_RDWR, 0600)
	}
	return
}

func (s *server) Run() error {
	if err := s.setupEnv(); err != nil {
		return err
	}
	if err := s.openDb(); err != nil {
		return err
	}

	err := s.openDevNode()
	if err != nil {
		return err
	}

	if err = s.startSocketServer(); err != nil {
		return err
	}

	if _, err = unix.Write(s.devnode, []byte("dir "+s.cfg.CachePath)); err != nil {
		return err
	} else if _, err = unix.Write(s.devnode, []byte("tag "+cacheTag)); err != nil {
		return err
	} else if _, err = unix.Write(s.devnode, []byte("bind ondemand")); err != nil {
		return err
	}

	wchan := make(chan []byte)
	for i := 0; i < s.cfg.Workers; i++ {
		go func() {
			for {
				s.handleMessage(<-wchan)
			}
		}()
	}

	fds := make([]unix.PollFd, 1)
	errors := 0
	for {
		if errors > 10 {
			// we might be spinning somehow, slow down
			time.Sleep(time.Duration(errors) * time.Millisecond)
		}
		fds[0] = unix.PollFd{Fd: int32(s.devnode), Events: unix.POLLIN}
		n, err := unix.Poll(fds, 3600*1000)
		if err != nil {
			log.Printf("error from poll: %v", err)
			errors++
			continue
		}
		if n != 1 {
			continue
		}
		// read until we get zero
		readAfterPoll := false
		for {
			buf := s.pool.Get().([]byte)
			n, err = unix.Read(s.devnode, buf)
			if err != nil {
				errors++
				log.Printf("error from read: %v", err)
				break
			} else if n == 0 {
				// handle bug in linux < 6.8 where poll returns POLLIN if there are any
				// outstanding requests, not just new ones
				if !readAfterPoll {
					log.Printf("empty read")
					errors++
				}
				break
			}
			readAfterPoll = true
			errors = 0
			wchan <- buf[:n]
		}
	}
	return nil
}

func (s *server) handleMessage(buf []byte) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
		if retErr != nil {
			log.Printf("error handling message: %v", retErr)
		}
		s.pool.Put(buf[0:CACHEFILES_MSG_MAX_SIZE])
	}()

	var r bytes.Reader
	r.Reset(buf)
	var msg cachefiles_msg
	if err := struc.Unpack(&r, &msg); err != nil {
		return err
	}
	switch msg.OpCode {
	case CACHEFILES_OP_OPEN:
		var open cachefiles_open
		if err := struc.Unpack(&r, &open); err != nil {
			return err
		}
		return s.handleOpen(msg.MsgId, msg.ObjectId, open.Fd, open.Flags, open.VolumeKey, open.CookieKey)
	case CACHEFILES_OP_CLOSE:
		return s.handleClose(msg.MsgId, msg.ObjectId)
	case CACHEFILES_OP_READ:
		var read cachefiles_read
		if err := struc.Unpack(&r, &read); err != nil {
			return err
		}
		return s.handleRead(msg.MsgId, msg.ObjectId, read.Len, read.Off)
	default:
		return errors.New("unknown opcode")
	}
}

func (s *server) handleOpen(msgId, objectId, fd, flags uint32, volume, cookie []byte) (retErr error) {
	// volume is "erofs,<domain_id>" (domain_id is same as fsid if not specified)
	// cookie is "<fsid>"
	// log.Println("OPEN", msgId, objectId, fd, flags, volume, cookie)

	var cacheSize int64

	defer func() {
		if retErr != nil {
			cacheSize = -int64(unix.ENODEV)
		}
		reply := fmt.Sprintf("copen %d,%d", msgId, cacheSize)
		if _, err := unix.Write(s.devnode, []byte(reply)); err != nil {
			log.Println("failed write to devnode", err)
		}
		if cacheSize < 0 {
			unix.Close(int(fd))
		}
	}()

	if string(volume) != "erofs,"+domainId+"\x00" {
		return fmt.Errorf("wrong domain %q", volume)
	}

	// slab or manifest
	cstr := string(cookie)
	if strings.HasPrefix(cstr, slabPrefix) {
		log.Println("OPEN SLAB", objectId, cstr)
		slabId, err := strconv.Atoi(cstr[len(slabPrefix):])
		if err != nil {
			return err
		}
		cacheSize, retErr = s.handleOpenSlab(msgId, objectId, fd, flags, truncU16(slabId))
		return
	} else if len(cstr) == 32 {
		log.Println("OPEN IMAGE", objectId, cstr)
		cacheSize, retErr = s.handleOpenImage(msgId, objectId, fd, flags, cstr)
		return
	} else {
		return fmt.Errorf("bad fsid %q", cstr)
	}
}

func (s *server) handleOpenSlab(msgId, objectId, fd, flags uint32, id uint16) (int64, error) {
	// record open state
	s.lock.Lock()
	defer s.lock.Unlock()
	s.cacheState[objectId] = &openFileState{
		fd:     fd,
		tp:     typeSlab,
		slabId: id,
	}
	return slabBytes, nil
}

func (s *server) handleOpenImage(msgId, objectId, fd, flags uint32, cookie string) (int64, error) {
	// check if we have this image
	var size int64
	err := s.db.Update(func(tx *bbolt.Tx) error {
		if rec := tx.Bucket(imageBucket).Get([]byte(cookie)); rec != nil {
			m, err := unmarshalAs[pb.DbImage](rec)
			if err != nil {
				return err
			}
			size = m.Size
		}
		return nil
	})
	if err != nil {
		return 0, err
	} else if size > 0 {
		s.lock.Lock()
		defer s.lock.Unlock()
		s.cacheState[objectId] = &openFileState{
			fd: fd,
			tp: typeImage,
		}
		return size, nil
	}

	// get manifest

	// check cached first
	gParams := s.cfg.Params.Params
	mReq := manifester.ManifestReq{
		Upstream:        s.cfg.Upstream,
		StorePathHash:   cookie,
		ChunkShift:      int(gParams.ChunkShift),
		DigestAlgo:      gParams.DigestAlgo,
		DigestBits:      int(gParams.DigestBits),
		SmallFileCutoff: s.cfg.SmallFileCutoff,
	}
	ctx := context.TODO()
	sb, err := s.mcread.Get(ctx, mReq.CacheKey(), nil)
	if err != nil {
		// not found cached, request it
		u := s.cfg.Params.ManifesterUrl + manifester.ManifestPath
		reqBytes, err := json.Marshal(mReq)
		if err != nil {
			return 0, err
		}
		res, err := http.Post(u, "application/json", bytes.NewReader(reqBytes))
		if err != nil {
			return 0, fmt.Errorf("manifester http error: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("manifester http status: %s", res.Status)
		} else if zr, err := zstd.NewReader(res.Body); err != nil {
			return 0, err
		} else if sb, err = io.ReadAll(zr); err != nil {
			return 0, err
		}
	}

	// verify signature and params
	entry, smParams, err := common.VerifyMessageAsEntry(s.cfg.StyxPubKeys, common.ManifestContext, sb)
	if err != nil {
		return 0, err
	}
	if smParams != nil {
		match := smParams.ChunkShift == gParams.ChunkShift &&
			smParams.DigestBits == gParams.DigestBits &&
			smParams.DigestAlgo == gParams.DigestAlgo
		if !match {
			return 0, fmt.Errorf("chunked manifest global params mismatch")
		}
	}

	// unmarshal into manifest
	data := entry.InlineData
	if len(data) == 0 {
		data, err = s.readChunkedData(entry)
	}

	var m pb.Manifest
	err = proto.Unmarshal(data, &m)
	if err != nil {
		return 0, fmt.Errorf("manifest unmarshal error: %w", err)
	}
	// transform manifest into image (allocate chunks)
	var image bytes.Buffer
	err = s.builder.BuildFromManifestWithSlab(&m, &image, s)
	if err != nil {
		return 0, fmt.Errorf("build image error: %w", err)
	}
	size = int64(image.Len())

	log.Printf("new image %s, %d bytes in manifest, %d bytes in erofs", cookie, len(sb), size)

	// record in db
	err = s.db.Update(func(tx *bbolt.Tx) error {
		buf, err := proto.Marshal(&pb.DbImage{Size: size})
		if err != nil {
			return err
		}
		return tx.Bucket(imageBucket).Put([]byte(cookie), buf)
	})
	if err != nil {
		return 0, err
	}

	// record open state
	s.lock.Lock()
	defer s.lock.Unlock()
	s.cacheState[objectId] = &openFileState{
		fd:        fd,
		tp:        typeImage,
		imageData: image.Bytes(),
	}

	return size, nil
}

func (s *server) handleClose(msgId, objectId uint32) error {
	log.Println("CLOSE", objectId)
	s.lock.Lock()
	defer s.lock.Unlock()
	if state := s.cacheState[objectId]; state != nil {
		unix.Close(int(state.fd))
		delete(s.cacheState, objectId)
	}
	return nil
}

func (s *server) handleRead(msgId, objectId uint32, ln, off uint64) error {
	s.lock.Lock()
	state := s.cacheState[objectId]
	s.lock.Unlock()

	if state == nil {
		panic("missing state")
	}

	defer func() {
		_, _, e1 := unix.Syscall(unix.SYS_IOCTL, uintptr(state.fd), CACHEFILES_IOC_READ_COMPLETE, uintptr(msgId))
		if e1 != 0 {
			fmt.Errorf("ioctl error %d", e1)
		}
	}()

	switch state.tp {
	case typeImage:
		log.Println("READ IMAGE", objectId, ln, off)
		return s.handleReadImage(state, ln, off)
	case typeSlab:
		log.Println("READ SLAB", objectId, ln, off)
		return s.handleReadSlab(state, ln, off)
	default:
		panic("bad state type")
	}
}

func (s *server) handleReadImage(state *openFileState, _, _ uint64) error {
	// always write whole thing
	off := int64(0)
	buf := state.imageData
	for len(buf) > 0 {
		toWrite := buf[:min(16<<10, len(buf))]
		n, err := unix.Pwrite(int(state.fd), toWrite, off)
		if err != nil {
			return err
		} else if n < len(toWrite) {
			return io.ErrShortWrite
		}
		buf = buf[n:]
		off += int64(n)
	}

	// FIXME: delete imageData since we don't need it anymore

	return nil
}

func (s *server) handleReadSlab(state *openFileState, ln, off uint64) error {
	ctx := context.TODO()

	digest := make([]byte, s.digestBytes)

	if ln > (1 << s.cfg.Params.Params.ChunkShift) {
		panic("got too big slab read")
	}

	err := s.db.View(func(tx *bbolt.Tx) error {
		sb := tx.Bucket(slabBucket).Bucket(slabKey(state.slabId))
		if sb == nil {
			return errors.New("missing slab bucket")
		}
		cur := sb.Cursor()
		target := addrKey(truncU32(off >> s.blockShift))
		k, v := cur.Seek(target)
		if k == nil {
			k, v = cur.Last()
		} else if !bytes.Equal(target, k) {
			k, v = cur.Prev()
		}
		if k == nil {
			return errors.New("ran off start of bucket")
		}
		// reset off so we write at the right place
		off = uint64(addrFromKey(k)) << s.blockShift
		copy(digest, v)
		return nil
	})
	if err != nil {
		return err
	}

	digestStr := base64.RawURLEncoding.EncodeToString(digest)

	// TODO: consider if this is the right scope for duplicate suppression
	_, err, _ = s.sf.Do(digestStr, func() (any, error) {
		buf := make([]byte, 1<<s.cfg.Params.Params.ChunkShift)
		got, err := s.csread.Get(ctx, digestStr, buf[:0])
		if err != nil {
			return nil, err
		}

		if len(got) > len(buf) {
			return nil, fmt.Errorf("chunk overflowed chunk size: %d > %d", len(got), len(buf))
		} else if uint64(s.blockShift.roundup(int64(len(got)))) < ln {
			return nil, fmt.Errorf("chunk underflowed requested len: %d < %d", len(got), ln)
		}

		n, err := unix.Pwrite(int(state.fd), buf, int64(off))
		if err == nil && n != len(buf) {
			err = fmt.Errorf("short write %d != %d (requested %d)", n, len(buf), ln)
		}
		return nil, err
	})

	return err
}

func (s *server) readChunkedData(entry *pb.Entry) ([]byte, error) {
	ctx := context.TODO()

	out := make([]byte, entry.Size)
	dest := out
	digests := entry.Digests

	var eg errgroup.Group
	eg.SetLimit(20) // TODO: configurable

	for len(digests) > 0 && len(dest) > 0 {
		digest := digests[:s.digestBytes]
		digests = digests[s.digestBytes:]
		toRead := min(len(dest), 1<<s.cfg.Params.Params.ChunkShift)
		buf := dest[:0]
		dest = dest[toRead:]

		// TODO: see if we can diff these chunks against some other chunks we have

		eg.Go(func() error {
			digestStr := base64.RawURLEncoding.EncodeToString(digest)
			got, err := s.csread.Get(ctx, digestStr, buf)
			if err != nil {
				return err
			} else if len(got) != toRead {
				return fmt.Errorf("chunk was wrong size: %d vs %d", len(got), toRead)
			}
			return nil
		})
	}
	return valOrErr(out, eg.Wait())
}

// slab manager

const (
	slabBytes  = 1 << 40
	slabBlocks = slabBytes >> 12
)

func slabKey(id uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, id)
	return b
}

func addrKey(addr uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, addr)
	return b
}

func addrFromKey(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}

func locValue(id uint16, addr uint32) []byte {
	loc := make([]byte, 6)
	binary.LittleEndian.PutUint16(loc, id)
	binary.LittleEndian.PutUint32(loc[2:], addr)
	return loc
}

func loadLoc(b []byte) (id uint16, addr uint32) {
	return binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint32(b[2:])
}

func (s *server) VerifyParams(hashBytes, blockSize, chunkSize int) error {
	if hashBytes != s.digestBytes || blockSize != int(s.blockShift.size()) || chunkSize != (1<<s.cfg.Params.Params.ChunkShift) {
		return errors.New("mismatched params")
	}
	return nil
}

func (s *server) AllocateBatch(blocks []uint16, digests []byte) ([]erofs.SlabLoc, error) {
	n := len(blocks)
	out := make([]erofs.SlabLoc, n)
	err := s.db.Update(func(tx *bbolt.Tx) error {
		cb, slabroot := tx.Bucket(chunkBucket), tx.Bucket(slabBucket)
		sb, err := slabroot.CreateBucketIfNotExists(slabKey(0))
		if err != nil {
			return err
		}

		seq := sb.Sequence()

		for i := range out {
			digest := digests[i*s.digestBytes : (i+1)*s.digestBytes]
			var id uint16
			var addr uint32
			if have := cb.Get(digest); have == nil { // allocate
				addr = truncU32(seq)
				seq += uint64(blocks[i])
				if err := cb.Put(digest, locValue(id, addr)); err != nil {
					return err
				} else if err = sb.Put(addrKey(addr), digest); err != nil {
					return err
				}
			} else {
				id, addr = loadLoc(have)
			}
			out[i].SlabId = id
			out[i].Addr = addr

			// TODO: check seq for overflow here and move to next slab
		}

		return sb.SetSequence(seq)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *server) SlabInfo(slabId uint16) (tag string, totalBlocks uint32) {
	return fmt.Sprintf("_slab_%d", slabId), slabBlocks
}
