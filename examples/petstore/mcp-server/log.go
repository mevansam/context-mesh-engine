// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mevansam/context-mesh-engine/examples/petstore/petstore-auth-server/jwtx"
)

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func fmtAud(aud jwt.ClaimStrings) string {
	if len(aud) == 0 {
		return ""
	}
	return strings.Join([]string(aud), ",")
}

func logClientToken(c *jwtx.ClientClaims) {
	exp := time.Time{}
	if c.ExpiresAt != nil {
		exp = c.ExpiresAt.Time
	}
	log.Printf("client JWT token_use=%s sub=%s iss=%s aud=%s exp=%s",
		c.TokenUse, c.Subject, c.Issuer, fmtAud(c.Audience), fmtTime(exp))
}

func logUserToken(u *jwtx.UserClaims) {
	exp := time.Time{}
	if u.ExpiresAt != nil {
		exp = u.ExpiresAt.Time
	}
	log.Printf("end-user JWT token_use=%s username=%s userStatus=%d sub=%s iss=%s aud=%s exp=%s",
		u.TokenUse, u.Username, u.UserStatus, u.Subject, u.Issuer, fmtAud(u.Audience), fmtTime(exp))
}

func logJSON(prefix string, v any) {
	if v == nil {
		log.Printf("%s <nil>", prefix)
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("%s %v", prefix, v)
		return
	}
	s := string(b)
	const max = 512
	if len(s) > max {
		s = s[:max] + fmt.Sprintf("…(%d bytes)", len(b))
	}
	log.Printf("%s %s", prefix, s)
}
