package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"shutterseek/tools/importer/internal/extractor"
)

// Walk traverses the photos directory and sends supported file paths to the channel.
// It skips video files if skipVideo is true.
func Walk(root string, skipVideo bool, paths chan<- string) error {
	defer close(paths)

	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip inaccessible paths
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil // skip hidden files
		}

		ext := strings.ToLower(filepath.Ext(path))
		if extractor.IsPhotoFormat(ext) {
			paths <- path
		} else if !skipVideo && extractor.IsVideoFormat(ext) {
			paths <- path
		}
		// else: skip unsupported files

		return nil
	})
}
