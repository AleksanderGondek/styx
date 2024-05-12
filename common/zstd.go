package common

import (
	"sync"

	"github.com/DataDog/zstd"
)

type ZstdCtxPool struct {
	p sync.Pool
}

func NewZstdCtxPool() *ZstdCtxPool {
	return &ZstdCtxPool{
		p: sync.Pool{New: func() any { return zstd.NewCtx() }},
	}
}

func (z *ZstdCtxPool) Get() zstd.Ctx {
	return z.p.Get().(zstd.Ctx)
}

func (z *ZstdCtxPool) Put(c zstd.Ctx) {
	z.p.Put(c)
}
