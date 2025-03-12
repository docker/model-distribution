package store

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/pkg/model"
)

var _ v1.Layer = &model.Layer{}
