// SPDX-FileCopyrightText: 2021 The NGI Pointer Secure-Scuttlebutt Team of 2020/2021
//
// SPDX-License-Identifier: MIT

package signinwithssb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	refs "github.com/ssbc/go-ssb-refs"
)

func TestPayloadString(t *testing.T) {

	server := refs.FeedRef{ID: bytes.Repeat([]byte{1}, 32), Algo: "test"}
	client := refs.FeedRef{ID: bytes.Repeat([]byte{2}, 32), Algo: "test"}

	var req ClientPayload

	req.ServerID = server
	req.ClientID = client

	req.ServerChallenge = "fooo"
	req.ClientChallenge = "barr"

	want := "=http-auth-sign-in:@AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=.test:@AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=.test:fooo:barr"

	got := req.createMessage()
	assert.Equal(t, want, string(got))
}

func TestGenerateAndDecode(t *testing.T) {
	r := require.New(t)

	b, err := DecodeChallengeString(GenerateChallenge())
	r.NoError(err)
	r.Len(b, challengeLength)

	b, err = DecodeChallengeString("toshort")
	r.Error(err)
	r.Nil(b)
}
