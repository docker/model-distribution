package store

import "sync"

// IncompleteFilesStack is a thread-safe stack for tracking incomplete files
type IncompleteFilesStack struct {
	files []string
	mu    sync.Mutex
}

// NewIncompleteFilesStack creates a new thread-safe stack for tracking incomplete files
func NewIncompleteFilesStack() *IncompleteFilesStack {
	return &IncompleteFilesStack{
		files: make([]string, 0),
	}
}

// Push adds a file to the stack
func (s *IncompleteFilesStack) Push(file string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, file)
}

// Pop removes the last added file from the stack
func (s *IncompleteFilesStack) Pop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.files) > 0 {
		s.files = s.files[:len(s.files)-1]
	}
}

// GetAll returns a copy of all files in the stack
func (s *IncompleteFilesStack) GetAll() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := make([]string, len(s.files))
	copy(files, s.files)
	return files
}
