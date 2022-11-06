// SPDX-FileCopyrightText: 2021 The NGI Pointer Secure-Scuttlebutt Team of 2020/2021
//
// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ssbc/go-muxrpc/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mindeco.de/http/auth"

	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb-room/v2/internal/maybemod/keys"
	"github.com/ssbc/go-ssb-room/v2/internal/signinwithssb"
	"github.com/ssbc/go-ssb-room/v2/roomdb"
	weberrors "github.com/ssbc/go-ssb-room/v2/web/errors"
	"github.com/ssbc/go-ssb-room/v2/web/router"
	"github.com/ssbc/go-ssb-room/v2/web/webassert"
)

func TestRestricted(t *testing.T) {
	ts := setup(t)
	a := assert.New(t)

	testURLs := []string{
		"/admin/",
		"/admin/anything/",
	}

	for _, tstr := range testURLs {
		turl, err := url.Parse(tstr)
		if err != nil {
			t.Fatal(err)
		}
		html, resp := ts.Client.GetHTML(turl)
		a.Equal(http.StatusForbidden, resp.Code, "wrong HTTP status code for %q", turl)
		found := html.Find("h1").Text()
		a.Equal("Error #403 - Forbidden", found, "wrong error message code for %q", turl)
	}
}

func TestLoginForm(t *testing.T) {
	ts := setup(t)
	a := assert.New(t)

	url := ts.URLTo(router.AuthFallbackLogin)

	html, resp := ts.Client.GetHTML(url)
	a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code")

	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AuthTitle"},
		{"#welcome", "AuthFallbackWelcome"},
	})
}

func TestFallbackAuthWrongPassword(t *testing.T) {
	ts := setup(t)
	a := assert.New(t)

	signInFormURL := ts.URLTo(router.AuthFallbackLogin)

	doc, resp := ts.Client.GetHTML(signInFormURL)
	a.Equal(http.StatusOK, resp.Code)

	csrfCookie := resp.Result().Cookies()
	a.True(len(csrfCookie) > 0, "should have one cookie for CSRF protection validation")

	passwordForm := doc.Find("#password-fallback")
	webassert.CSRFTokenPresent(t, passwordForm)

	csrfTokenElem := passwordForm.Find("input[type=hidden]")
	a.Equal(1, csrfTokenElem.Length())

	csrfName, has := csrfTokenElem.Attr("name")
	a.True(has, "should have a name attribute")

	csrfValue, has := csrfTokenElem.Attr("value")
	a.True(has, "should have value attribute")

	loginVals := url.Values{
		"user": []string{"test"},
		"pass": []string{"wong"},

		csrfName: []string{csrfValue},
	}
	ts.AuthFallbackDB.CheckReturns(nil, weberrors.ErrRedirect{
		Path:   "/fallback/login",
		Reason: auth.ErrBadLogin,
	})

	signInURL := ts.URLTo(router.AuthFallbackFinalize)

	// important for CSRF
	var refererHeader = make(http.Header)
	refererHeader.Set("Referer", "https://localhost")
	ts.Client.SetHeaders(refererHeader)

	resp = ts.Client.PostForm(signInURL, loginVals)
	a.Equal(http.StatusSeeOther, resp.Code, "wrong HTTP status code for sign in")
	a.Equal(1, ts.AuthFallbackDB.CheckCallCount())

	// check flash error for bad login
	res := resp.Result()
	a.Equal(signInFormURL.Path, res.Header.Get("Location"), "redirecting to overview")
	a.True(len(res.Cookies()) > 0, "got a cookie (flash msg)")

	html, resp := ts.Client.GetHTML(signInFormURL)
	a.Equal(http.StatusOK, resp.Code)

	flashes := html.Find("#flashes-list").Children()
	a.Equal(1, flashes.Length())
	a.Equal("ErrorAuthBadLogin", flashes.Text())
}

func TestFallbackAuthWorks(t *testing.T) {
	ts := setup(t)
	a := assert.New(t)

	signInFormURL := ts.URLTo(router.AuthFallbackLogin)

	doc, resp := ts.Client.GetHTML(signInFormURL)
	a.Equal(http.StatusOK, resp.Code)

	csrfCookie := resp.Result().Cookies()
	a.True(len(csrfCookie) > 0, "should have one cookie for CSRF protection validation")

	passwordForm := doc.Find("#password-fallback")
	webassert.CSRFTokenPresent(t, passwordForm)

	csrfTokenElem := passwordForm.Find("input[type=hidden]")
	a.Equal(1, csrfTokenElem.Length())

	csrfName, has := csrfTokenElem.Attr("name")
	a.True(has, "should have a name attribute")

	csrfValue, has := csrfTokenElem.Attr("value")
	a.True(has, "should have value attribute")

	loginVals := url.Values{
		"user": []string{"test"},
		"pass": []string{"test"},

		csrfName: []string{csrfValue},
	}
	ts.AuthFallbackDB.CheckReturns(int64(23), nil)

	signInURL := ts.URLTo(router.AuthFallbackFinalize)

	// important for CSRF
	var refererHeader = make(http.Header)
	refererHeader.Set("Referer", "https://localhost")
	ts.Client.SetHeaders(refererHeader)

	resp = ts.Client.PostForm(signInURL, loginVals)
	a.Equal(http.StatusSeeOther, resp.Code, "wrong HTTP status code for sign in")

	a.Equal(1, ts.AuthFallbackDB.CheckCallCount())

	// now request the protected dashboard page
	dashboardURL := ts.URLTo(router.AdminDashboard)

	html, resp := ts.Client.GetHTML(dashboardURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code for dashboard") {
		t.Log(html.Find("body").Text())
	}

	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AdminDashboardTitle"},
	})

	testRef, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte{0}, 32), refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Error(err)
	}
	ts.RoomState.AddEndpoint(testRef, nil)

	html, resp = ts.Client.GetHTML(dashboardURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code") {
		t.Log(html.Find("body").Text())
	}
	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AdminDashboardTitle"},
	})

	testRef2, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte{1}, 32), refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Error(err)
	}
	ts.RoomState.AddEndpoint(testRef2, nil)

	html, resp = ts.Client.GetHTML(dashboardURL)
	a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code")

	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AdminDashboardTitle"},
	})
}

func TestAuthWithSSBClientInitNotConnected(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the client is a member but not connected right now
	ts.MembersDB.GetByFeedReturns(roomdb.Member{ID: 1234}, nil)
	ts.MockedEndpoints.GetEndpointForReturns(nil, false)

	client, err := keys.NewKeyPair(nil)
	r.NoError(err)

	cc := signinwithssb.GenerateChallenge()

	signInStartURL := ts.URLTo(router.AuthWithSSBLogin,
		"cid", client.Feed.String(),
		"cc", cc,
	)
	r.NotNil(signInStartURL)
	doc, resp := ts.Client.GetHTML(signInStartURL)
	a.Equal(http.StatusForbidden, resp.Code)

	webassert.Localized(t, doc, []webassert.LocalizedElement{
		// {"#welcome", "AuthWithSSBWelcome"},
		// {"title", "AuthWithSSBTitle"},
	})
}

func TestAuthWithSSBClientInitNotAllowed(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the client isnt a member
	ts.MembersDB.GetByFeedReturns(roomdb.Member{}, roomdb.ErrNotFound)
	ts.MockedEndpoints.GetEndpointForReturns(nil, false)

	client, err := keys.NewKeyPair(nil)
	r.NoError(err)

	cc := signinwithssb.GenerateChallenge()

	signInStartURL := ts.URLTo(router.AuthWithSSBLogin,
		"cid", client.Feed.String(),
		"cc", cc,
	)
	r.NotNil(signInStartURL)

	doc, resp := ts.Client.GetHTML(signInStartURL)
	a.Equal(http.StatusForbidden, resp.Code)
	t.Log(resp.Body.String())

	webassert.Localized(t, doc, []webassert.LocalizedElement{
		// {"#welcome", "AuthWithSSBWelcome"},
		// {"title", "AuthWithSSBTitle"},
	})
}

func TestAuthWithSSBClientAlternativeRoute(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the client isnt a member
	ts.MembersDB.GetByFeedReturns(roomdb.Member{}, roomdb.ErrNotFound)
	ts.MockedEndpoints.GetEndpointForReturns(nil, false)

	client, err := keys.NewKeyPair(nil)
	r.NoError(err)

	cc := signinwithssb.GenerateChallenge()

	loginURL := ts.URLTo(router.AuthLogin,
		"ssb-http-auth", 1,
		"cid", client.Feed.String(),
		"cc", cc,
	)
	r.NotNil(loginURL)

	t.Log(loginURL.String())
	doc, resp := ts.Client.GetHTML(loginURL)
	t.Log()
	a.Equal(http.StatusForbidden, resp.Code)

	webassert.Localized(t, doc, []webassert.LocalizedElement{
		// {"#welcome", "AuthWithSSBWelcome"},
		// {"title", "AuthWithSSBTitle"},
	})
}

func TestAuthWithSSBClientInitHasClient(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the request to be signed later
	var payload signinwithssb.ClientPayload
	payload.ServerID = ts.NetworkInfo.RoomID

	// the keypair for our client
	testMember := roomdb.Member{ID: 1234}
	client, err := keys.NewKeyPair(nil)
	r.NoError(err)
	testMember.PubKey = client.Feed

	// setup the mocked database
	ts.MembersDB.GetByFeedReturns(testMember, nil)
	ts.AuthWithSSB.CreateTokenReturns("abcdefgh", nil)
	ts.AuthWithSSB.CheckTokenReturns(testMember.ID, nil)
	ts.MembersDB.GetByIDReturns(testMember, nil)

	// fill the basic infos of the request
	payload.ClientID = client.Feed

	// this is our fake "connected" client
	var edp muxrpc.FakeEndpoint

	// setup a mocked muxrpc call that asserts the arguments and returns the needed signature
	edp.AsyncCalls(func(_ context.Context, ret interface{}, encoding muxrpc.RequestEncoding, method muxrpc.Method, args ...interface{}) error {
		a.Equal(muxrpc.TypeString, encoding)
		a.Equal("httpAuth.requestSolution", method.String())

		r.Len(args, 2, "expected two args")

		serverChallenge, ok := args[0].(string)
		r.True(ok, "argument[0] is not a string: %T", args[0])
		a.NotEqual("", serverChallenge)
		// update the challenge
		payload.ServerChallenge = serverChallenge

		clientChallenge, ok := args[1].(string)
		r.True(ok, "argument[1] is not a string: %T", args[1])
		a.Equal(payload.ClientChallenge, clientChallenge)

		strptr, ok := ret.(*string)
		r.True(ok, "return is not a string pointer: %T", ret)

		// sign the request now that we have the sc
		clientSig := payload.Sign(client.Pair.Secret)

		*strptr = base64.StdEncoding.EncodeToString(clientSig)
		return nil
	})

	// setup the fake client endpoint
	ts.MockedEndpoints.GetEndpointForReturns(&edp, true)

	cc := signinwithssb.GenerateChallenge()
	// update the challenge
	payload.ClientChallenge = cc

	// prepare the url

	signInStartURL := ts.URLTo(router.AuthWithSSBLogin,
		"cid", client.Feed.String(),
		"cc", cc,
	)
	signInStartURL.Host = "localhost"
	signInStartURL.Scheme = "https"

	r.NotNil(signInStartURL)

	t.Log(signInStartURL.String())
	doc, resp := ts.Client.GetHTML(signInStartURL)
	a.Equal(http.StatusTemporaryRedirect, resp.Code)

	dashboardURL := ts.URLTo(router.AdminDashboard)
	a.Equal(dashboardURL.Path, resp.Header().Get("Location"))

	webassert.Localized(t, doc, []webassert.LocalizedElement{
		// {"#welcome", "AuthWithSSBWelcome"},
		// {"title", "AuthWithSSBTitle"},
	})

	// analyse the endpoints call
	a.Equal(1, ts.MockedEndpoints.GetEndpointForCallCount())
	edpRef := ts.MockedEndpoints.GetEndpointForArgsForCall(0)
	a.Equal(client.Feed.String(), edpRef.String())

	// check the mock was called
	a.Equal(1, edp.AsyncCallCount())

	// check that we have a new cookie
	sessionCookie := resp.Result().Cookies()
	r.True(len(sessionCookie) > 0, "expecting one cookie!")

	html, resp := ts.Client.GetHTML(dashboardURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code for dashboard") {
		t.Log(html.Find("body").Text())
	}

	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AdminDashboardTitle"},
	})
}

func TestAuthWithSSBServerInitHappyPath(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the keypair for our client
	testMember := roomdb.Member{ID: 1234}
	client, err := keys.NewKeyPair(nil)
	r.NoError(err)
	testMember.PubKey = client.Feed

	// setup the mocked database
	ts.MembersDB.GetByFeedReturns(testMember, nil)

	// prepare the url

	signInStartURL := ts.URLTo(router.AuthWithSSBLogin,
		"cid", client.Feed.String(),
	)
	r.NotNil(signInStartURL)

	html, resp := ts.Client.GetHTML(signInStartURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code for dashboard") {
		t.Log(html.Find("body").Text())
	}

	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AuthWithSSBTitle"},
		{"#welcome", "AuthWithSSBWelcome"},
	})

	jsFile, has := html.Find("script").Attr("src")
	a.True(has, "should have client code")
	a.Equal("/assets/auth-withssb-uri.js", jsFile)

	serverChallenge, has := html.Find("#challenge").Attr("data-sc")
	a.True(has, "should have server challenge")
	a.NotEqual("", serverChallenge)

	ssbURI, has := html.Find("#start-auth-uri").Attr("href")
	a.True(has, "should have an ssb:experimental uri")
	a.True(strings.HasPrefix(ssbURI, "ssb:experimental?"), "not an ssb-uri? %s", ssbURI)

	parsedURI, err := url.Parse(ssbURI)
	r.NoError(err)
	a.Equal("ssb", parsedURI.Scheme)
	a.Equal("experimental", parsedURI.Opaque)

	qry := parsedURI.Query()
	a.Equal("start-http-auth", qry.Get("action"))
	a.Equal(serverChallenge, qry.Get("sc"))
	a.Equal(ts.NetworkInfo.RoomID.String(), qry.Get("sid"))
	a.Equal(ts.NetworkInfo.MultiserverAddress(), qry.Get("multiserverAddress"))

	qrCode, has := html.Find("#start-auth-qrcode").Attr("src")
	a.True(has, "should have the inline image data")
	a.True(strings.HasPrefix(qrCode, "data:image/png;base64,"))

	// TODO: decode image data and check qr code(?)

	// simulate muxrpc client
	testToken := "our-test-token"
	ts.AuthWithSSB.CheckTokenReturns(23, nil)
	go func() {
		time.Sleep(4 * time.Second)
		err = ts.SignalBridge.SessionWorked(serverChallenge, testToken)
		r.NoError(err)
	}()

	// start reading sse
	sseURL := ts.URLTo(router.AuthWithSSBServerEvents, "sc", serverChallenge)
	resp = ts.Client.GetBody(sseURL)
	a.Equal(http.StatusOK, resp.Result().StatusCode)

	// check contents of sse channel
	sseBody := resp.Body.String()

	a.True(strings.Contains(sseBody, "data: Waiting for solution"), "ping data")
	a.True(strings.Contains(sseBody, "event: ping\n"), "ping event")

	wantDataToken := fmt.Sprintf("data: %s\n", testToken)
	a.True(strings.Contains(sseBody, wantDataToken), "token data")
	a.True(strings.Contains(sseBody, "event: success\n"), "success event")

	// use the token and go to /withssb/finalize and get a cookie
	// (this happens in the browser engine via login-events.js)
	finalizeURL := ts.URLTo(router.AuthWithSSBFinalize, "token", testToken)

	resp = ts.Client.GetBody(finalizeURL)

	// now request the protected dashboard page
	dashboardURL := ts.URLTo(router.AdminDashboard)

	html, resp = ts.Client.GetHTML(dashboardURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code for dashboard") {
		t.Log(html.Find("body").Text())
	}

	webassert.Localized(t, html, []webassert.LocalizedElement{
		{"title", "AdminDashboardTitle"},
	})
}

func TestAuthWithSSBServerInitWrongSolution(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the keypair for our client
	testMember := roomdb.Member{ID: 1234}
	client, err := keys.NewKeyPair(nil)
	r.NoError(err)
	testMember.PubKey = client.Feed

	// setup the mocked database
	ts.MembersDB.GetByFeedReturns(testMember, nil)

	// prepare the url
	signInStartURL := ts.URLTo(router.AuthWithSSBLogin,
		"cid", client.Feed.String(),
	)
	r.NotNil(signInStartURL)

	html, resp := ts.Client.GetHTML(signInStartURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code for dashboard") {
		t.Log(html.Find("body").Text())
	}

	serverChallenge, has := html.Find("#challenge").Attr("data-sc")
	a.True(has, "should have server challenge")
	a.NotEqual("", serverChallenge)

	// simulate muxrpc client
	ts.AuthWithSSB.CheckTokenReturns(-1, roomdb.ErrNotFound)
	go func() {
		time.Sleep(4 * time.Second)
		err = ts.SignalBridge.SessionFailed(serverChallenge, fmt.Errorf("wrong solution"))
		r.NoError(err)
	}()

	// start reading sse
	sseURL := ts.URLTo(router.AuthWithSSBServerEvents, "sc", serverChallenge)
	resp = ts.Client.GetBody(sseURL)
	a.Equal(http.StatusOK, resp.Result().StatusCode)

	// check contents of sse channel
	sseBody := resp.Body.String()

	a.True(strings.Contains(sseBody, "data: Waiting for solution"), "ping data")
	a.True(strings.Contains(sseBody, "event: ping\n"), "ping event")

	a.True(strings.Contains(sseBody, "data: wrong solution\n"), "reason data")
	a.True(strings.Contains(sseBody, "event: failed\n"), "success event")

	// use an invalid token
	finalizeURL := ts.URLTo(router.AuthWithSSBFinalize, "token", "wrong")
	resp = ts.Client.GetBody(finalizeURL)
	a.Equal(http.StatusForbidden, resp.Result().StatusCode)
}

func TestAuthWithSSBServerOnAndroidChrome(t *testing.T) {
	ts := setup(t)
	a, r := assert.New(t), require.New(t)

	// the keypair for our client
	testMember := roomdb.Member{ID: 1234}
	client, err := keys.NewKeyPair(nil)
	r.NoError(err)
	testMember.PubKey = client.Feed

	// Mimic Android Chrome
	var uaHeader = make(http.Header)
	uaHeader.Set("User-Agent", "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5 Build/MOB30H) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/44.0.2403.133 Mobile Safari/537.36")
	ts.Client.SetHeaders(uaHeader)

	// setup the mocked database
	ts.MembersDB.GetByFeedReturns(testMember, nil)

	// prepare the url
	signInStartURL := ts.URLTo(router.AuthWithSSBLogin,
		"cid", client.Feed.String(),
	)
	r.NotNil(signInStartURL)

	html, resp := ts.Client.GetHTML(signInStartURL)
	if !a.Equal(http.StatusOK, resp.Code, "wrong HTTP status code for dashboard") {
		t.Log(html.Find("body").Text())
	}

	serverChallenge, has := html.Find("#challenge").Attr("data-sc")
	a.True(has, "should have server challenge")
	a.NotEqual("", serverChallenge)

	ssbURI, has := html.Find("#start-auth-uri").Attr("href")
	a.True(has, "should have an Android Intent URI")
	a.True(strings.HasPrefix(ssbURI, "intent://experimental"), "not an Android Intent URI? %s", ssbURI)

	parsedURI, err := url.Parse(ssbURI)
	r.NoError(err)
	a.Equal("intent", parsedURI.Scheme)
	a.Equal("experimental", parsedURI.Host)

	qry := parsedURI.Query()
	a.Equal("start-http-auth", qry.Get("action"))

	frag := parsedURI.Fragment
	a.Equal("Intent;scheme=ssb;end;", frag)
}
