package store

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/pkg/partial"
)

var _ v1.Layer = &partial.Layer{}
