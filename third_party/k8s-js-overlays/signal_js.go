//go:build js

/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package debugger

import "os"

// compareSignal is the signal to trigger cache compare. GOOS=js has no
// SIGUSR2 (syscall.SIGUSR2 does not exist for this GOOS -- that's the
// entire reason this file exists, see ../README.md); mirrors
// signal_windows.go's os.Interrupt fallback, which is already a no-op
// placeholder signal on a GOOS lacking SIGUSR2.
var compareSignal os.Signal = os.Interrupt
