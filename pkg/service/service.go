package service

import (
	"context"
)

type Service interface {
	run(
		ctx context.Context,
		running chan struct{},
		stopping chan struct{},
		stopped chan struct{},
	)
}
