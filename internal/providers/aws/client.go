// Package aws implements regional Amazon EC2 accounts using the Query API.
package aws

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
)

type credential struct {
	AccessKey      string   `json:"access_key_id"`
	SecretKey      string   `json:"secret_access_key"`
	SessionToken   string   `json:"session_token,omitempty"`
	Region         string   `json:"region"`
	Subnet         string   `json:"subnet_id"`
	Groups         []string `json:"security_group_ids"`
	ManagedNetwork bool     `json:"managed_network,omitempty"`
}

// Commercial partition only; opt-in/availability is verified against the API.
var regionPattern = regexp.MustCompile(`^(us|eu|ap|ca|sa|af|me|il|mx)-(east|west|south|north|central|southeast|northeast)-[1-9][0-9]?$`)
var idPattern = regexp.MustCompile(`^(i|ami|subnet|sg|vpc)-[0-9a-f]{8}([0-9a-f]{9})?$`)
var accountPattern = regexp.MustCompile(`^[0-9]{12}$`)

func problem(class providers.ErrorClass, op, code string, status int) *providers.Error {
	return cloudutil.Problem("AWS", class, op, code, status)
}
func signingCredentials(ctx context.Context, s providers.CredentialSource) (credential, error) {
	var c credential
	if cloudutil.ReadCredential(ctx, s, &c) != nil || !regionPattern.MatchString(c.Region) || c.AccessKey == "" || c.SecretKey == "" || strings.ContainsAny(c.AccessKey+c.SecretKey+c.SessionToken, "\r\n\x00") {
		return c, problem(providers.ErrorAuthentication, "credential", "INVALID_CREDENTIAL_JSON", 0)
	}
	return c, nil
}
func credentials(ctx context.Context, s providers.CredentialSource) (credential, error) {
	c, e := signingCredentials(ctx, s)
	if e != nil {
		return c, e
	}
	if !idPattern.MatchString(c.Subnet) || !strings.HasPrefix(c.Subnet, "subnet-") || len(c.Groups) > 5 || (len(c.Groups) == 0 && !c.ManagedNetwork) || (c.ManagedNetwork && len(c.Groups) > 0) {
		return c, problem(providers.ErrorInvalidRequest, "credential", "NETWORK_SELECTION_REQUIRED", 0)
	}
	seen := map[string]bool{}
	for _, g := range c.Groups {
		if !idPattern.MatchString(g) || !strings.HasPrefix(g, "sg-") || seen[g] {
			return c, problem(providers.ErrorAuthentication, "credential", "INVALID_SECURITY_GROUPS", 0)
		}
		seen[g] = true
	}
	return c, nil
}

type client struct {
	http          *http.Client
	credentials   providers.CredentialSource
	region        string
	expected      string
	scopeMu       sync.Mutex
	scopeVerified bool
}

func hash(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func mac(key []byte, s string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s))
	return h.Sum(nil)
}
func encode(q url.Values) string { return strings.ReplaceAll(q.Encode(), "+", "%20") }
func sign(req *http.Request, body string, c credential, service string, now time.Time) {
	date := now.UTC().Format("20060102")
	stamp := now.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", stamp)
	headers := "host:" + req.URL.Host + "\nx-amz-date:" + stamp + "\n"
	signed := "host;x-amz-date"
	if c.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", c.SessionToken)
		headers += "x-amz-security-token:" + c.SessionToken + "\n"
		signed += ";x-amz-security-token"
	}
	canonical := req.Method + "\n/\n" + req.URL.RawQuery + "\n" + headers + "\n" + signed + "\n" + hash(body)
	scope := date + "/" + c.Region + "/" + service + "/aws4_request"
	key := mac(mac(mac(mac([]byte("AWS4"+c.SecretKey), date), c.Region), service), "aws4_request")
	signature := hex.EncodeToString(mac(key, "AWS4-HMAC-SHA256\n"+stamp+"\n"+scope+"\n"+hash(canonical)))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.AccessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+signature)
}
func (c *client) call(ctx context.Context, service, action string, q url.Values, mutation bool, out any) error {
	cred, e := signingCredentials(ctx, c.credentials)
	if e != nil {
		return e
	}
	if cred.Region != c.region {
		return problem(providers.ErrorAuthentication, action, "REGION_SCOPE_CHANGED", 0)
	}
	if service == "ec2" && c.expected != "" {
		if e := c.verifyScope(ctx); e != nil {
			return e
		}
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("Action", action)
	version := "2016-11-15"
	if service == "sts" {
		version = "2011-06-15"
	}
	q.Set("Version", version)
	method, body := http.MethodGet, ""
	endpoint := "https://" + service + "." + c.region + ".amazonaws.com/"
	if mutation {
		method = http.MethodPost
		body = encode(q)
	} else {
		endpoint += "?" + encode(q)
	}
	req, e := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(body))
	if e != nil {
		return problem(providers.ErrorInvalidRequest, action, "INVALID_REQUEST", 0)
	}
	if mutation {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	sign(req, body, cred, service, time.Now())
	resp, e := c.http.Do(req)
	if e != nil {
		return problem(providers.ErrorTransport, action, "TRANSPORT_ERROR", 0)
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if e != nil || len(raw) > 8<<20 {
		return problem(providers.ErrorUnavailable, action, "INVALID_RESPONSE_BODY", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classify(action, resp.StatusCode, raw)
	}
	var root struct{ XMLName xml.Name }
	if xml.Unmarshal(raw, &root) != nil || root.XMLName.Local != action+"Response" {
		return problem(providers.ErrorUnavailable, action, "INVALID_RESPONSE_ENVELOPE", resp.StatusCode)
	}
	if out != nil && xml.Unmarshal(raw, out) != nil {
		return problem(providers.ErrorUnavailable, action, "INVALID_RESPONSE", resp.StatusCode)
	}
	return nil
}
func classify(op string, status int, b []byte) *providers.Error {
	var envelope struct {
		Errors []struct {
			Code string `xml:"Code"`
		} `xml:"Errors>Error"`
		Error struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}
	_ = xml.Unmarshal(b, &envelope)
	code := envelope.Error.Code
	if len(envelope.Errors) > 0 {
		code = envelope.Errors[0].Code
	}
	class, label := providers.ErrorUnknown, "REQUEST_FAILED"
	switch code {
	case "AuthFailure", "InvalidClientTokenId", "InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken", "ExpiredTokenException", "RequestExpired":
		class, label = providers.ErrorAuthentication, "AUTHENTICATION_FAILED"
	case "UnauthorizedOperation", "AccessDenied", "AccessDeniedException", "OperationNotPermitted":
		class, label = providers.ErrorPermissionDenied, "PERMISSION_DENIED"
	case "RequestLimitExceeded", "Throttling", "ThrottlingException":
		class, label = providers.ErrorRateLimited, "RATE_LIMITED"
	case "InstanceLimitExceeded", "VcpuLimitExceeded":
		class, label = providers.ErrorCapacity, "COMPUTE_QUOTA_EXCEEDED"
	case "InsufficientInstanceCapacity", "InsufficientHostCapacity", "Unsupported":
		class, label = providers.ErrorRegionCapacity, "REGION_CAPACITY_UNAVAILABLE"
	case "InvalidAMIID.NotFound", "InvalidAMIID.Unavailable":
		class, label = providers.ErrorImageUnavailable, "IMAGE_UNAVAILABLE"
	case "InvalidInstanceID.NotFound":
		class, label = providers.ErrorNotFound, "INSTANCE_NOT_FOUND"
	case "IdempotentParameterMismatch":
		class, label = providers.ErrorAmbiguousOutcome, "IDEMPOTENT_INTENT_MISMATCH"
	case "OptInRequired", "PendingVerification":
		class, label = providers.ErrorPermissionDenied, "ACCOUNT_ACCESS_REQUIRED"
	default:
		switch {
		case status == 401:
			class, label = providers.ErrorAuthentication, "AUTHENTICATION_FAILED"
		case status == 403:
			class, label = providers.ErrorPermissionDenied, "PERMISSION_DENIED"
		case status == 429:
			class, label = providers.ErrorRateLimited, "RATE_LIMITED"
		case status >= 500 || status >= 300 && status < 400:
			class, label = providers.ErrorUnavailable, "PROVIDER_UNAVAILABLE"
		case status >= 400 && status < 500:
			class, label = providers.ErrorInvalidRequest, "REQUEST_REJECTED"
		}
	}
	return problem(class, op, label, status)
}

// Every page must have the expected collection. Repeated tokens and duplicate
// identities fail the entire read, never producing a false empty inventory.
func pages[T any](ctx context.Context, c *client, action, set string, q url.Values, id func(T) string) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	pageSize := "1000"
	if action == "DescribeInstanceTypes" {
		pageSize = "100"
	}
	q.Set("MaxResults", pageSize)
	out := []T{}
	tokens := map[string]bool{}
	ids := map[string]bool{}
	for n := 0; n < 1000; n++ {
		var x struct {
			Next string `xml:"nextToken"`
			Raw  string `xml:",innerxml"`
		}
		if e := c.call(ctx, "ec2", action, q, false, &x); e != nil {
			return nil, e
		}
		decoder := xml.NewDecoder(strings.NewReader("<root>" + x.Raw + "</root>"))
		found := false
		for {
			t, e := decoder.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, problem(providers.ErrorUnavailable, action, "INVALID_PAGE", 200)
			}
			st, ok := t.(xml.StartElement)
			if !ok || st.Name.Local != set {
				continue
			}
			if found {
				return nil, problem(providers.ErrorUnavailable, action, "DUPLICATE_SET", 200)
			}
			found = true
			var collection struct {
				Items []T `xml:"item"`
			}
			if decoder.DecodeElement(&collection, &st) != nil {
				return nil, problem(providers.ErrorUnavailable, action, "INVALID_COLLECTION", 200)
			}
			for _, item := range collection.Items {
				key := id(item)
				if key == "" || ids[key] {
					return nil, problem(providers.ErrorUnavailable, action, "INVALID_COLLECTION_IDENTITY", 200)
				}
				ids[key] = true
				out = append(out, item)
			}
		}
		if !found {
			return nil, problem(providers.ErrorUnavailable, action, "MISSING_COLLECTION", 200)
		}
		if x.Next == "" {
			return out, nil
		}
		if tokens[x.Next] {
			return nil, problem(providers.ErrorUnavailable, action, "PAGINATION_LOOP", 200)
		}
		tokens[x.Next] = true
		q.Set("NextToken", x.Next)
	}
	return nil, problem(providers.ErrorUnavailable, action, "PAGINATION_LIMIT", 200)
}

func (c *client) verifyScope(ctx context.Context) error {
	c.scopeMu.Lock()
	defer c.scopeMu.Unlock()
	if c.scopeVerified {
		return nil
	}
	var x struct {
		Result struct {
			Account string `xml:"Account"`
		} `xml:"GetCallerIdentityResult"`
	}
	if e := c.call(ctx, "sts", "GetCallerIdentity", nil, false, &x); e != nil {
		return e
	}
	if !accountPattern.MatchString(x.Result.Account) || x.Result.Account+":"+c.region != c.expected {
		return problem(providers.ErrorAuthentication, "account", "ACCOUNT_SCOPE_MISMATCH", 0)
	}
	c.scopeVerified = true
	return nil
}
