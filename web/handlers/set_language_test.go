// SPDX-FileCopyrightText: 2021 The NGI Pointer Secure-Scuttlebutt Team of 2020/2021
//
// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ssbc/go-ssb-room/v2/web/i18n"
	"github.com/ssbc/go-ssb-room/v2/web/router"
)

func TestLanguageDefaultNoCookie(t *testing.T) {
	ts := setup(t)
	a := assert.New(t)
	route := ts.URLTo(router.CompleteIndex)

	html, res := ts.Client.GetHTML(route)
	a.Equal(http.StatusOK, res.Code, "wrong HTTP status code")

	languageForms := html.Find("#visitor-set-language form")
	// two languages: english, deutsch => two <form> elements
	a.Equal(2, languageForms.Length())

	// verify there is no language cookie to set yet
	cookieHeader := res.Header()["Set-Cookie"]
	for _, cookie := range cookieHeader {
		cookieName := strings.Split(cookie, "=")[0]
		a.NotEqual(cookieName, i18n.LanguageCookieName)
	}
}

func TestLanguageChooseGerman(t *testing.T) {
	ts := setup(t)
	a := assert.New(t)
	route := ts.URLTo(router.CompleteIndex)
	postEndpoint := ts.URLTo(router.CompleteSetLanguage)

	html, res := ts.Client.GetHTML(route)
	a.Equal(http.StatusOK, res.Code, "wrong HTTP status code")

	csrfTokenElem := html.Find(`#visitor-set-language input[name="gorilla.csrf.Token"]`)
	a.Equal(2, csrfTokenElem.Length())

	csrfName, has := csrfTokenElem.First().Attr("name")
	a.True(has, "should have a name attribute")

	csrfValue, has := csrfTokenElem.First().Attr("value")
	a.True(has, "should have value attribute")

	// construct the post request fields, simulating picking a language
	setLanguageFields := url.Values{
		"lang":   []string{"de"},
		"page":   []string{"/"},
		csrfName: []string{csrfValue},
	}

	// set the referer header (important! otherwise our nicely crafted request yields a 500 :'()
	var refererHeader = make(http.Header)
	refererHeader.Set("Referer", "https://localhost")
	ts.Client.SetHeaders(refererHeader)

	// send the post request
	postRes := ts.Client.PostForm(postEndpoint, setLanguageFields)
	a.Equal(http.StatusSeeOther, postRes.Code, "wrong HTTP status code for sign in")

	// verify there is one language cookie to set
	cookieHeader := postRes.Header()["Set-Cookie"]
	var languageCookies int
	for _, cookie := range cookieHeader {
		cookieName := strings.Split(cookie, "=")[0]
		if cookieName == i18n.LanguageCookieName {
			languageCookies += 1
		}
	}
	a.Equal(1, languageCookies, "should have one language cookie set after posting")
}
