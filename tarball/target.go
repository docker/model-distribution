package tarball

import (
	"archive/tar"
	"context"
	"fmt"
	"github.com/docker/model-distribution/types"
	"github.com/google/go-containerregistry/pkg/name"
	"io"
	"os"
	"path/filepath"
)

type Target struct {
	reference name.Tag
	writer    io.Writer
}

func (t *Target) Write(ctx context.Context, mdl types.ModelArtifact, progressWriter io.Writer) error {
	//defer t.writer.Close()

	//pr := progress.NewProgressReporter(progressWriter, progress.PushMsg, nil)
	//defer pr.Wait()

	tw := tar.NewWriter(t.writer)
	defer tw.Close()

	rm, err := mdl.RawManifest()
	if err != nil {
		return err
	}

	if err := tw.WriteHeader(&tar.Header{
		Name:     "blobs",
		Typeflag: tar.TypeDir,
	}); err != nil {
		return err
	}

	ls, err := mdl.Layers()
	if err != nil {
		return err
	}
	algDirs := map[string]struct{}{}
	for _, layer := range ls {
		dgst, err := layer.Digest()
		if err != nil {
			return err
		}
		_, ok := algDirs[dgst.Algorithm]
		if !ok {
			if err = tw.WriteHeader(&tar.Header{
				Name:     filepath.Join("blobs", dgst.Algorithm),
				Typeflag: tar.TypeDir,
			}); err != nil {
				return err
			}
			algDirs[dgst.Algorithm] = struct{}{}
		}
		sz, err := layer.Size()
		if err != nil {
			return err
		}
		if err = tw.WriteHeader(&tar.Header{
			Name: filepath.Join("blobs", dgst.Algorithm, dgst.Hex),
			Mode: 0666,
			Size: sz,
		}); err != nil {
			return err
		}
		rc, err := layer.Uncompressed()
		if err != nil {
			return err
		}
		defer rc.Close()
		if _, err = io.Copy(tw, rc); err != nil {
			return err
		}
	}
	rcf, err := mdl.RawConfigFile()
	if err != nil {
		return err
	}
	cn, err := mdl.ConfigName()
	if err != nil {
		return err
	}
	if err = tw.WriteHeader(&tar.Header{
		Name: filepath.Join("blobs", cn.Algorithm, cn.Hex),
		Mode: 0666,
		Size: int64(len(rcf)),
	}); err != nil {
		return err
	}
	if _, err = tw.Write(rcf); err != nil {
		return fmt.Errorf("write config blob contents: %w", err)
	}

	if err := tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(rm)),
		Mode: 0666,
	}); err != nil {
		return fmt.Errorf("write manifest.json header: %w", err)
	}
	if _, err = tw.Write(rm); err != nil {
		return fmt.Errorf("write manifest.json contents: %w", err)
	}

	return nil
}

func NewFileTarget(tag string, path string) (*Target, error) {
	var ref name.Tag
	if tag != "" {
		ref, err := name.NewTag(tag)
		if err != nil {
			return nil, fmt.Errorf("invalid tag: %q: %w", ref, err)
		}
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

func NewTarget(w io.Writer) (*Target, error) {
	return &Target{
		writer: w,
	}, nil
}
