package notmuch

// Copyright © 2015 The go.notmuch Authors. Authors can be found in the AUTHORS file.
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.

// #cgo LDFLAGS: -lnotmuch
// #include <stdlib.h>
// #include <notmuch.h>
import "C"

// ConfigList represents the (key, value) configuration pairs of a database,
// including those from the configuration file and built-in defaults.
type ConfigList cStruct

func (cl *ConfigList) Close() error {
	return (*cStruct)(cl).doClose(func() error {
		C.notmuch_config_pairs_destroy(cl.toC())
		return nil
	})
}

func (cl *ConfigList) toC() *C.notmuch_config_pairs_t {
	return (*C.notmuch_config_pairs_t)(cl.cptr)
}

// Next retrieves the next config pair from the ConfigList.
// Neither key, nor value may be nil, or this function will panic.
// Next returns true if a pair was successfully retrieved.
func (cl *ConfigList) Next(key, value *string) bool {
	for cl.valid() {
		*key = cl.key()
		*value = cl.value()
		C.notmuch_config_pairs_move_to_next(cl.toC())
		if *value != "" {
			return true
		}
	}
	return false
}

func (cl *ConfigList) valid() bool {
	cbool := C.notmuch_config_pairs_valid(cl.toC())
	return int(cbool) != 0
}

func (cl *ConfigList) key() string {
	cstr := C.notmuch_config_pairs_key(cl.toC())
	if cstr == nil {
		// this should never happen
		return ""
	}
	return C.GoString(cstr)
}

func (cl *ConfigList) value() string {
	cstr := C.notmuch_config_pairs_value(cl.toC())
	if cstr == nil {
		return ""
	}
	return C.GoString(cstr)
}
