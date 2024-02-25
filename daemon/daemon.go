package daemon

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sync"
	"time"

	"github.com/lunixbochs/struc"
	"go.etcd.io/bbolt"
	"golang.org/x/sys/unix"

	"github.com/dnr/styx/erofs"
	"github.com/dnr/styx/manifester"
)

type (
	server struct {
		cfg  *Config
		db   *bbolt.DB
		pool *sync.Pool
	}

	Config struct {
		DevPath   string
		CachePath string

		ManifesterUrl  string
		ChunkStoreRead manifester.ChunkStoreRead
	}
)

var _ erofs.SlabManager = (*server)(nil)

func CachefilesServer(cfg Config) *server {
	return &server{
		cfg:  &cfg,
		pool: &sync.Pool{New: func() any { return make([]byte, CACHEFILES_MSG_MAX_SIZE) }},
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
		if _, err := tx.CreateBucketIfNotExists(metaBucket); err != nil {
			return err
		} else if _, err = tx.CreateBucketIfNotExists(chunkBucket); err != nil {
			return err
		}
		return nil
	})
}

func (s *server) startSocketServer() (err error) {
	socketPath := path.Join(s.cfg.CachePath, Socket)
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
	return os.MkdirAll(s.cfg.CachePath, 0755)
}

func (s *server) openDevNode() (devnode int, err error) {
	devnode, err = unix.Open(s.cfg.DevPath, unix.O_RDWR, 0600)
	if err == unix.ENOENT {
		_ = unix.Mknod(s.cfg.DevPath, 0600|unix.S_IFCHR, 10*256+122)
		devnode, err = unix.Open(s.cfg.DevPath, unix.O_RDWR, 0600)
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

	devnode, err := s.openDevNode()
	if err != nil {
		return err
	}

	if err = s.startSocketServer(); err != nil {
		return err
	}

	if _, err = unix.Write(devnode, []byte("dir "+s.cfg.CachePath)); err != nil {
		return err
	} else if _, err = unix.Write(devnode, []byte("tag "+cacheTag)); err != nil {
		return err
	} else if _, err = unix.Write(devnode, []byte("bind ondemand")); err != nil {
		return err
	}

	fds := make([]unix.PollFd, 1)
	errors := 0
	for {
		if errors > 100 {
			// we might be spinning somehow, slow down
			time.Sleep(time.Duration(errors) * time.Millisecond)
		}
		fds[0] = unix.PollFd{Fd: int32(devnode), Events: unix.POLLIN}
		n, err := unix.Poll(fds, 3600*1000)
		if err != nil {
			log.Printf("error from poll: %v", err)
			errors++
			continue
		}
		if n != 1 {
			continue
		}
		buf := s.pool.Get().([]byte)
		n, err = unix.Read(devnode, buf)
		if err != nil {
			errors++
			log.Printf("error from read: %v", err)
			continue
		}
		errors = 0
		go s.handleMessage(buf, n)
	}
	return nil
}

func (s *server) handleMessage(_buf []byte, _n int) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
		if retErr != nil {
			log.Printf("error handling message: %v", retErr)
		}
		s.pool.Put(_buf)
	}()

	buf := _buf[:_n]
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

func (s *server) handleOpen(msgId, objectId, fd, flags uint32, volume, cookie []byte) error {
	// volume is "erofs,<domain_id>" (domain_id is same as fsid if not specified)
	// cookie is "<fsid>"
	log.Println("OPEN", msgId, objectId, fd, flags, string(volume), string(cookie))
	panic("unimpl")
}

func (s *server) handleClose(msgId, objectId uint32) error {
	log.Println("CLOSE", msgId, objectId)
	panic("unimpl")
}

func (s *server) handleRead(msgId, objectId uint32, ln, off uint64) error {
	log.Println("READ", msgId, objectId, ln, off)
	panic("unimpl")
}

// slab manager

const slabSize = 1 << (40 - 12)

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
	if hashBytes != fHashBytes || blockSize != fBlockSize || chunkSize == fChunkSize {
		return errors.New("mismatched params")
	}
	return nil
}

func (s *server) AllocateBatch(blocks []uint16, digests []byte) ([]erofs.SlabLoc, error) {
	n := len(blocks)
	out := make([]erofs.SlabLoc, n)
	err := s.db.Update(func(tx *bbolt.Tx) error {
		cb, sb := tx.Bucket(chunkBucket), tx.Bucket(slabBucket).Bucket(slabKey(0))

		seq := sb.Sequence()

		for i := range out {
			digest := digests[i*fHashBytes : (i+1)*fHashBytes]
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
		}

		return sb.SetSequence(seq)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *server) SlabInfo(slabId uint16) (tag string, totalBlocks uint32) {
	return fmt.Sprintf("styx-slab-%d", slabId), slabSize
}