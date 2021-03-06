package cas

import (
	"context"
	"errors"
	"github.com/tmsong/hlog"
	"net/http"
	"strconv"
	"time"
)

type key int

const ( // emulating enums is actually pretty ugly in go.
	clientKey key = iota
	authenticationResponseKey
)

// setClient associates a Client with a http.Request.
func SetClient(r *http.Request, c *Client) {
	ctx := context.WithValue(r.Context(), clientKey, c)
	r2 := r.WithContext(ctx)
	*r = *r2
}

//setClientWithLogger associates a new logger client with a http.Request
func SetClientWithLogger(r *http.Request, c *Client, l *hlog.Logger) {
	newCli := &Client{c.cli, l}
	newCli.stValidator = NewServiceTicketValidator(c.stValidator.client,
		c.stValidator.casURL, c.stValidator.validationType, newCli)
	newCli.pmValidator = NewPermissionValidator(c.pmValidator.client,
		c.pmValidator.permissionURL, newCli)
	newCli.SetSessionStore(c.sessions.CopyWithParent(newCli))
	newCli.SetTicketStore(c.tickets.CopyWithParent(newCli))
	ctx := context.WithValue(r.Context(), clientKey, newCli)
	r2 := r.WithContext(ctx)
	*r = *r2
}

// getClient retrieves the Client associated with the http.Request.
func GetClient(r *http.Request) *Client {
	if c := r.Context().Value(clientKey); c != nil {
		return c.(*Client)
	}

	return nil // explicitly pass along the nil to caller -- conforms to previous impl
}

// RedirectToLogin allows CAS protected handlers to redirect a request
// to the CAS login page.
func RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	c := GetClient(r)
	if c == nil {
		err := "cas: redirect to cas failed as no client associated with request"
		http.Error(w, err, http.StatusInternalServerError)
		return
	}

	c.RedirectToLogin(w, r)
}

// RedirectToLogout allows CAS protected handlers to redirect a request
// to the CAS logout page.
func RedirectToLogout(w http.ResponseWriter, r *http.Request) {
	c := GetClient(r)
	if c == nil {
		err := "cas: redirect to cas failed as no client associated with request"
		http.Error(w, err, http.StatusInternalServerError)
		return
	}

	c.RedirectToLogout(w, r)
}

// setAuthenticationResponse associates an AuthenticationResponse with
// a http.Request.
func setAuthenticationResponse(r *http.Request, a *AuthenticationResponse) {
	ctx := context.WithValue(r.Context(), authenticationResponseKey, a)
	r2 := r.WithContext(ctx)
	*r = *r2
}

// getAuthenticationResponse retrieves the AuthenticationResponse associated
// with a http.Request.
func getAuthenticationResponse(r *http.Request) *AuthenticationResponse {
	if a := r.Context().Value(authenticationResponseKey); a != nil {
		return a.(*AuthenticationResponse)
	}

	return nil // explicitly pass along the nil to caller -- conforms to previous impl
}

// IsAuthenticated indicates whether the request has been authenticated with CAS.
func IsAuthenticated(r *http.Request) bool {
	if a := getAuthenticationResponse(r); a != nil {
		return true
	}

	return false
}

// Username returns the authenticated users username
func Username(r *http.Request) string {
	if a := getAuthenticationResponse(r); a != nil {
		return a.User
	}

	return ""
}

// Attributes returns the authenticated users attributes.
func Attributes(r *http.Request) UserAttributes {
	if a := getAuthenticationResponse(r); a != nil {
		return a.Attributes
	}

	return nil
}

// Attrs returns the authenticated users attributes.
func GetUserAttrs(r *http.Request) *UserAttrs {
	if a := getAuthenticationResponse(r); a != nil {
		tmp := &UserAttrsStruct{}
		InterfaceToStruct(a.Attributes, &tmp)
		if ret, err := tmp.ToUserAttrs(); err != nil {
			return nil
		} else {
			return ret
		}
	}
	return nil
}

// AuthenticationDate returns the date and time that authentication was performed.
//
// This may return time.IsZero if Authentication Date information is not included
// in the CAS service validation response. This will be the case for CAS 2.0
// protocol servers.
func AuthenticationDate(r *http.Request) time.Time {
	var t time.Time
	if a := getAuthenticationResponse(r); a != nil {
		t = a.AuthenticationDate
	}

	return t
}

// IsNewLogin indicates whether the CAS service ticket was granted following a
// new authentication.
//
// This may incorrectly return false if Is New Login information is not included
// in the CAS service validation response. This will be the case for CAS 2.0
// protocol servers.
func IsNewLogin(r *http.Request) bool {
	if a := getAuthenticationResponse(r); a != nil {
		return a.IsNewLogin
	}

	return false
}

// IsRememberedLogin indicates whether the CAS service ticket was granted by the
// presence of a long term authentication token.
//
// This may incorrectly return false if Remembered Login information is not included
// in the CAS service validation response. This will be the case for CAS 2.0
// protocol servers.
func IsRememberedLogin(r *http.Request) bool {
	if a := getAuthenticationResponse(r); a != nil {
		return a.IsRememberedLogin
	}

	return false
}

// MemberOf returns the list of groups which the user belongs to.
func MemberOf(r *http.Request) []string {
	if a := getAuthenticationResponse(r); a != nil {
		return a.MemberOf
	}

	return nil
}

// MemberOf returns the list of groups which the user belongs to.
func GetCurrentUserId(r *http.Request) int64 {
	if a := getAuthenticationResponse(r); a != nil {
		if a.Attributes != nil && len(a.Attributes) > 0 {
			if v, ok := a.Attributes["uid"]; ok {
				uid, err := strconv.ParseInt(v[0].(string), 10, 64)
				if err != nil {
					return 0
				}
				return uid
			}
		}
		return 0
	}
	return 0
}

// MemberOf returns the list of groups which the user belongs to.
func SetCurrentUserId(r *http.Request, userId int64) {
	if a := getAuthenticationResponse(r); a == nil {
		setAuthenticationResponse(r, &AuthenticationResponse{})
	}
	a := getAuthenticationResponse(r)
	if a.Attributes == nil {
		a.Attributes = make(UserAttributes)
	}
	a.Attributes["uid"] = []interface{}{strconv.FormatInt(userId, 10)}
	return
}

func HasPermission(r *http.Request) bool {
	c := GetClient(r)
	if c == nil {
		return false
	}
	if c.PermissionValidateForRequest(r) != nil {
		return false
	}
	return true
}

func RoleList(r *http.Request) ([]RoleListResponse, error) {
	c := GetClient(r)
	if c == nil {
		return nil, errors.New("no client associated with request")
	}
	return c.RoleList(r)
}

func PermissionList(r *http.Request, roleId int64) ([]PermissionListResponse, error) {
	c := GetClient(r)
	if c == nil {
		return nil, errors.New("no client associated with request")
	}
	return c.PermissionList(r, roleId)
}

func UserInfo(r *http.Request, userId int64) (*UserInfoResponse, error) {
	c := GetClient(r)
	if c == nil {
		return nil, errors.New("no client associated with request")
	}
	return c.UserInfo(userId)
}

func UserInfoDetail(r *http.Request, userId int64) (*UserInfoResponse, error) {
	c := GetClient(r)
	if c == nil {
		return nil, errors.New("no client associated with request")
	}
	return c.UserInfo(userId)
}

func DepartmentInfo(r *http.Request, userId int64) (*UserInfoResponse, error) {
	c := GetClient(r)
	if c == nil {
		return nil, errors.New("no client associated with request")
	}
	return c.UserInfo(userId)
}
