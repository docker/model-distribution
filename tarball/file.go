package tarball

import (
	"context"
	"fmt"
	"github.com/docker/model-distribution/types"
	"io"
	"os"
)

type FileTarget struct {
	path string
}

func (t *FileTarget) Write(ctx context.Context, mdl types.ModelArtifact, pw io.Writer) error {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("create file for archive: %w", err)
	}
	defer f.Close()
	target, err := NewTarget(f)
	if err != nil {
		return fmt.Errorf("create target: %w", err)
	}
	return target.Write(ctx, mdl, pw)
}

func NewFileTarget(path string) *FileTarget {
	return &FileTarget{
		path: path,
	}
}
