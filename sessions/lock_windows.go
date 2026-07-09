// Copyright 2025 gurtcli authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package sessions

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func lockDB(cfgDir string, readOnly bool) (func(), error) {
	path := filepath.Join(cfgDir, "sessions.db.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	ol := &windows.Overlapped{}

	// Exclusive lock for writes, shared lock for reads
	var flags uint32 = windows.LOCKFILE_FAIL_IMMEDIATELY
	if !readOnly {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}

	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, fmt.Errorf("LockFileEx: %w", err)
	}

	return func() {
		windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		f.Close()
	}, nil
}
