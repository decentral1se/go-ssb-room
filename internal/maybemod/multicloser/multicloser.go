// SPDX-FileCopyrightText: 2021 The NGI Pointer Secure-Scuttlebutt Team of 2020/2021
//
// SPDX-License-Identifier: MIT

package multicloser

import (
	"fmt"
	"io"
	"sync"

	"github.com/ssbc/go-ssb-room/v2/internal/maybemod/multierror"
)

type Closer struct {
	cs []io.Closer
	l  sync.Mutex
}

func (mc *Closer) Add(c io.Closer) {
	mc.l.Lock()
	defer mc.l.Unlock()

	mc.cs = append(mc.cs, c)
}

var _ io.Closer = (*Closer)(nil)

func (mc *Closer) Close() error {
	mc.l.Lock()
	defer mc.l.Unlock()

	var (
		hasErrs bool
		errs    []error
	)

	for i, c := range mc.cs {
		if cerr := c.Close(); cerr != nil {
			cerr = fmt.Errorf("Closer: c%d failed: %w", i, cerr)
			errs = append(errs, cerr)
			hasErrs = true
		}
	}

	if !hasErrs {
		return nil
	}

	return multierror.List{Errs: errs}
}
