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

//go:build unix

package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func lockDB(cfgDir string, readOnly bool) (func(), error) {
	path := filepath.Join(cfgDir, "sessions.db.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	kind := syscall.LOCK_EX
	if readOnly {
		kind = syscall.LOCK_SH
	}
	if err := syscall.Flock(int(f.Fd()), kind); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
