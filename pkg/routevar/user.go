package routevar

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"sourcegraph.com/sourcegraph/sourcegraph/api/sourcegraph"
)

var (
	// User captures UserSpec strings in URL routes.
	User = `{User:` + namedToNonCapturingGroups(UserPattern) + `}`

	// Person captures PersonSpec strings in URL routes.
	Person = `{Person:` + namedToNonCapturingGroups(UserPattern) + `}`
)

// UserPattern is the regexp pattern that matches UserSpec strings:
// "login" or "1$" (for UID 1).
const UserPattern = `(?:(?P<uid>\d+\$)|(?P<login>[\w-][\w.-]*))`

var (
	userPattern = regexp.MustCompile("^" + UserPattern + "$")
)

// parseUser parses a UserSpec string. If spec is invalid, an
// InvalidError is returned.
func parseUser(spec string) (uid uint32, login string, err error) {
	if m := userPattern.FindStringSubmatch(spec); m != nil {
		uidStr := m[1]
		if uidStr != "" {
			var uid64 uint64
			uid64, err = strconv.ParseUint(strings.TrimSuffix(uidStr, "$"), 10, 32)
			if err != nil {
				return 0, "", InvalidError{"UserSpec", spec, err}
			}
			uid = uint32(uid64)
		}
		login = m[2]
		return
	}
	return 0, "", InvalidError{"UserSpec", spec, nil}
}

// userString returns a UserSpec string. It is the inverse of
// ParseUser. It does not check the validity of the inputs.
func userString(uid uint32, login string) string {
	if uid != 0 {
		return fmt.Sprintf("%d$", uid)
	}
	return login
}

func UserString(s sourcegraph.UserSpec) string {
	return userString(uint32(s.UID), s.Login)
}

func UserRouteVars(s sourcegraph.UserSpec) map[string]string {
	return map[string]string{"User": UserString(s)}
}

// ParseUserSpec parses a string generated by UserString and returns
// the equivalent UserSpec struct.
func ParseUserSpec(s string) (sourcegraph.UserSpec, error) {
	uid, login, err := parseUser(s)
	if err != nil {
		return sourcegraph.UserSpec{}, err
	}
	return sourcegraph.UserSpec{
		UID:   int32(uid),
		Login: login,
	}, nil
}
