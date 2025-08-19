package bundle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/model-distribution/types"
)

func Unpack(dir string, mdl types.Model) error {
	if err := unpackGGUFs(dir, mdl); err != nil {
		return fmt.Errorf("add GGUF file(s) to runtime bundle: %w", err)
	}
	if err := unpackMultiModalProjector(dir, mdl); err != nil {
		return fmt.Errorf("add GGUF file(s) to runtime bundle: %w", err)
	}
	return nil
}

func unpackGGUFs(dir string, mdl types.Model) error {
	ggufPaths, err := mdl.GGUFPaths()
	if err != nil {
		return err
	}

	if len(ggufPaths) == 1 {
		return unpackFile(filepath.Join(dir, "model.gguf"), ggufPaths[0])
	}

	for i := range ggufPaths {
		name := fmt.Sprintf("model-%05d-of-%05d.gguf", i+1, len(ggufPaths))
		if err := unpackFile(filepath.Join(dir, name), ggufPaths[i]); err != nil {
			return err
		}
	}

	return nil
}

func unpackMultiModalProjector(dir string, mdl types.Model) error {
	path, err := mdl.MMPROJPath()
	if err != nil {
		return nil // no such file
	}
	return unpackFile(filepath.Join(dir, "model.mmproj"), path)
}

func unpackFile(bundlePath string, srcPath string) error {
	if err := os.Link(bundlePath, srcPath); err != nil {
		// if hardlink fails fall back to copy
		r, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer r.Close()

		w, err := os.Create(bundlePath)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, r)
		if err != nil {
			return err
		}
	}
	return nil
}
