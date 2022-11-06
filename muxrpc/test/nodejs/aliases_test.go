// SPDX-FileCopyrightText: 2021 The NGI Pointer Secure-Scuttlebutt Team of 2020/2021
//
// SPDX-License-Identifier: MIT

package nodejs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ssbc/go-ssb-room/v2/internal/aliases"
	"github.com/ssbc/go-ssb-room/v2/roomdb"
	"github.com/ssbc/go-ssb-room/v2/roomdb/mockdb"
)

func TestGoServerJSClientAliases(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil)

	var membersDB = &mockdb.FakeMembersService{}
	var aliasesDB = &mockdb.FakeAliasesService{}
	srv := ts.startGoServer(membersDB, aliasesDB)
	// allow all peers (there arent any we dont want to allow)
	membersDB.GetByFeedReturns(roomdb.Member{ID: 1234}, nil)

	// setup mocks for this test
	aliasesDB.RegisterReturns(nil)

	alice := ts.startJSClient("alice", "./testscripts/modern_aliases.js",
		srv.Network.GetListenAddr(),
		srv.Whoami(),
	)

	// the revoke call checks who the alias belongs to
	aliasesDB.ResolveReturns(roomdb.Alias{
		Name: "alice",
		Feed: alice,
	}, nil)

	time.Sleep(5 * time.Second)

	// wait for both to exit
	ts.wait()

	r.Equal(1, aliasesDB.RegisterCallCount(), "register call count")
	_, name, ref, signature := aliasesDB.RegisterArgsForCall(0)
	a.Equal("alice", name, "wrong alias registered")
	a.Equal(alice.String(), ref.String())

	var aliasReq aliases.Confirmation
	aliasReq.Alias = name
	aliasReq.Signature = signature
	aliasReq.UserID = alice
	aliasReq.RoomID = srv.Whoami()

	a.True(aliasReq.Verify(), "signature validation")

	r.Equal(1, aliasesDB.RevokeCallCount(), "revoke call count")
	_, name = aliasesDB.RevokeArgsForCall(0)
	a.Equal("alice", name, "wrong alias revoked")

}
