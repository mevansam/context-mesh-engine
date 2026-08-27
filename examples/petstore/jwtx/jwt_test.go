// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package jwtx

import (
	"testing"
	"time"
)

func TestSignParseClientAndUser(t *testing.T) {
	secret := []byte("petstore-demo-hs256")
	ctok, err := SignClient(secret, "petstore-mcp", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cc, err := ParseClient(secret, ctok)
	if err != nil {
		t.Fatal(err)
	}
	if cc.Subject != "petstore-mcp" || cc.TokenUse != TokenUseClient {
		t.Fatalf("%#v", cc)
	}
	utok, err := SignUser(secret, "buyer", 2, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc, err := ParseUser(secret, utok)
	if err != nil {
		t.Fatal(err)
	}
	if uc.Username != "buyer" || uc.UserStatus != 2 {
		t.Fatalf("%#v", uc)
	}
	if _, err := ParseUser(secret, ctok); err == nil {
		t.Fatal("client token accepted as user")
	}
}
