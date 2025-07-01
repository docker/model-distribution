package tar

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/model-distribution/internal/progress"
	"github.com/docker/model-distribution/types"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type Target struct {
	reference name.Reference
	writer    io.WriteCloser
}

func (t *Target) Write(ctx context.Context, mdl types.ModelArtifact, progressWriter io.Writer) error {
	defer t.writer.Close()

	pr := progress.NewProgressReporter(progressWriter, progress.PushMsg, nil)
	defer pr.Wait()

	if err := tarball.Write(t.reference, mdl, t.writer,
		tarball.WithProgress(pr.Updates()),
	); err != nil {
		return fmt.Errorf("write to tarball %q: %w", t.reference.String(), err)
	}
	return nil
}

func NewTarget(tag string, path string) (*Target, error) {
	ref, err := name.NewTag(tag)
	if err != nil {
		return nil, fmt.Errorf("invalid tag: %q: %w", ref, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("error creating tar archive at path %q: %w", path, err)
	}
	return &Target{
		reference: ref,
		writer:    f,
	}, nil
}
