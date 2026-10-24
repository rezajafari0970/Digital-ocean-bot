package aws

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"golang.org/x/crypto/ssh"
)

type testCredential string

func (c testCredential) Get(context.Context) ([]byte, error) { return []byte(c), nil }

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const fixtureCredential = `{"access_key_id":"fixture-access","secret_access_key":"fixture-secret","session_token":"fixture-session","region":"eu-west-1","subnet_id":"subnet-12345678","security_group_ids":["sg-12345678"]}`

func openTest(t *testing.T, fn transport) *Driver {
	t.Helper()
	x, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "account-a", HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.pricing.us-east-1.amazonaws.com" {
			return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return fn(r)
	})}, Credentials: testCredential(fixtureCredential)})
	if e != nil {
		t.Fatal(e)
	}
	return x.(*Driver)
}
func response(action, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<" + action + "Response>" + body + "</" + action + "Response>"))}
}
func query(r *http.Request) url.Values {
	if r.Method == "GET" {
		return r.URL.Query()
	}
	b, _ := io.ReadAll(r.Body)
	q, _ := url.ParseQuery(string(b))
	return q
}
func publicKey(t *testing.T) string {
	t.Helper()
	p, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	k, _ := ssh.NewPublicKey(p)
	return string(ssh.MarshalAuthorizedKey(k))
}

const imageXML = `<imagesSet><item><imageId>ami-12345678</imageId><name>ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20261001</name><imageOwnerId>099720109477</imageOwnerId><creationDate>2026-10-01T00:00:00Z</creationDate><imageState>available</imageState><architecture>x86_64</architecture><rootDeviceName>/dev/sda1</rootDeviceName><rootDeviceType>ebs</rootDeviceType><virtualizationType>hvm</virtualizationType><isPublic>true</isPublic><blockDeviceMapping><item><deviceName>/dev/sda1</deviceName><ebs><volumeSize>8</volumeSize></ebs></item></blockDeviceMapping></item></imagesSet>`

func fixtureRead(action string) string {
	switch action {
	case "GetCallerIdentity":
		return `<GetCallerIdentityResult><Account>123456789012</Account><Arn>arn:aws:iam::123456789012:user/fixture</Arn></GetCallerIdentityResult>`
	case "DescribeSubnets":
		return `<subnetSet><item><subnetId>subnet-12345678</subnetId><vpcId>vpc-12345678</vpcId><state>available</state><availabilityZone>eu-west-1a</availabilityZone></item></subnetSet>`
	case "DescribeSecurityGroups":
		return `<securityGroupInfo><item><groupId>sg-12345678</groupId><vpcId>vpc-12345678</vpcId></item></securityGroupInfo>`
	case "DescribeImages":
		return imageXML
	case "DescribeInstanceTypes":
		return `<instanceTypeSet><item><instanceType>t3.micro</instanceType><currentGeneration>true</currentGeneration><supportedRootDeviceTypes><item>ebs</item></supportedRootDeviceTypes><supportedVirtualizationTypes><item>hvm</item></supportedVirtualizationTypes><supportedBootModes><item>legacy-bios</item><item>uefi</item></supportedBootModes><vCpuInfo><defaultVCpus>2</defaultVCpus></vCpuInfo><memoryInfo><sizeInMiB>1024</sizeInMiB></memoryInfo><processorInfo><supportedArchitectures><item>x86_64</item></supportedArchitectures></processorInfo><supportedUsageClasses><item>on-demand</item></supportedUsageClasses></item></instanceTypeSet>`
	case "DescribeInstanceTypeOfferings":
		return `<instanceTypeOfferingSet><item><instanceType>t3.micro</instanceType><currentGeneration>true</currentGeneration><supportedRootDeviceTypes><item>ebs</item></supportedRootDeviceTypes><supportedVirtualizationTypes><item>hvm</item></supportedVirtualizationTypes><supportedBootModes><item>legacy-bios</item><item>uefi</item></supportedBootModes><location>eu-west-1a</location></item></instanceTypeOfferingSet>`
	case "DescribeInstances":
		return `<reservationSet/>`
	}
	return ""
}
func TestAWSCreateContractAndAmbiguousOutcome(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "lost_response"}[ambiguous], func(t *testing.T) {
			posts := 0
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "ec2.eu-west-1.amazonaws.com" || r.Header.Get("X-Amz-Security-Token") != "fixture-session" || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
					t.Fatal("unsigned or misrouted request")
				}
				q := query(r)
				a := q.Get("Action")
				if a != "RunInstances" {
					if r.Method != "GET" {
						t.Fatal("read fenced as mutation")
					}
					return response(a, fixtureRead(a)), nil
				}
				posts++
				if r.Method != "POST" {
					t.Fatal("unfenced mutation")
				}
				expected := map[string]string{"MinCount": "1", "MaxCount": "1", "ImageId": "ami-12345678", "NetworkInterface.1.SubnetId": "subnet-12345678", "NetworkInterface.1.SecurityGroupId.1": "sg-12345678", "NetworkInterface.1.AssociatePublicIpAddress": "true", "BlockDeviceMapping.1.Ebs.DeleteOnTermination": "true", "MetadataOptions.HttpTokens": "required", "TagSpecification.1.Tag.3.Value": "account-a", "TagSpecification.1.Tag.4.Value": "dob-deployment-fixed"}
				for k, v := range expected {
					if q.Get(k) != v {
						t.Errorf("%s=%s", k, q.Get(k))
					}
				}
				if len(q.Get("ClientToken")) != 64 {
					t.Fatal("unstable token")
				}
				data, e := base64.StdEncoding.DecodeString(q.Get("UserData"))
				if e != nil || !strings.Contains(string(data), `"disable_root":false`) || !strings.Contains(string(data), `"ssh_pwauth":false`) {
					t.Fatal("invalid bootstrap")
				}
				if ambiguous {
					return nil, errors.New("connection lost fixture-secret")
				}
				return response(a, `<instancesSet><item><instanceId>i-12345678</instanceId></item></instancesSet>`), nil
			})
			got, e := d.CreateServer(context.Background(), providers.CreateServerRequest{Name: "server", Identity: "dob-deployment-fixed", RegionID: "eu-west-1", PlanID: "t3.micro", ImageID: "ami-12345678", SSHAuthorizedKeys: []string{publicKey(t)}})
			if posts != 1 {
				t.Fatal("unexpected mutation retries", posts)
			}
			if ambiguous {
				if got.Outcome != providers.OutcomeAmbiguous || !providers.IsClass(e, providers.ErrorAmbiguousOutcome) || strings.Contains(e.Error(), "fixture-secret") {
					t.Fatal(got, e)
				}
			} else if e != nil || got.ServerID != "eu-west-1:i-12345678" {
				t.Fatal(got, e)
			}
		})
	}
}
func TestAWSInventoryFailsClosed(t *testing.T) {
	for _, body := range []string{"", `<reservationSet/><nextToken>loop</nextToken>`, `<reservationSet><item><reservationId>r-1</reservationId></item></reservationSet>`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			d := openTest(t, func(r *http.Request) (*http.Response, error) {
				calls++
				return response("DescribeInstances", body), nil
			})
			if xs, e := d.ListServers(context.Background()); e == nil || xs != nil || calls > 2 {
				t.Fatal(xs, e, calls)
			}
		})
	}
}
func TestAWSAccountCatalogAndScope(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		q := query(r)
		a := q.Get("Action")
		if a == "DescribeInstanceTypes" && q.Get("MaxResults") != "100" {
			t.Fatal("EC2 instance-type page exceeds API maximum", q.Get("MaxResults"))
		}
		return response(a, fixtureRead(a)), nil
	})
	a, e := d.Account(context.Background())
	if e != nil || a.ID != "123456789012:eu-west-1" {
		t.Fatal(a, e)
	}
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Images) != 1 || len(c.Plans) != 1 || c.Images[0].IDsByRegion["eu-west-1"] != "ami-12345678" {
		t.Fatal(c, e)
	}
	if _, e = d.GetServer(context.Background(), "us-east-1:i-12345678"); !providers.IsClass(e, providers.ErrorInvalidRequest) {
		t.Fatal(e)
	}
}
func TestAWSDeleteOwnershipAndFenceTarget(t *testing.T) {
	posts := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			posts++
		}
		return response("DescribeInstances", `<reservationSet><item><reservationId>r-1</reservationId><instancesSet><item><instanceId>i-12345678</instanceId><instanceState><name>running</name></instanceState></item></instancesSet></item></reservationSet>`), nil
	})
	if e := d.DeleteServer(context.Background(), "eu-west-1:i-12345678"); !providers.IsClass(e, providers.ErrorPermissionDenied) || posts != 0 {
		t.Fatal(e, posts)
	}
	body := "Action=TerminateInstances&InstanceId.1=i-12345678&Version=2016-11-15"
	r, _ := http.NewRequest("POST", "https://ec2.eu-west-1.amazonaws.com/", strings.NewReader(body))
	if TerminationTarget(r) != "eu-west-1:i-12345678" {
		t.Fatal("cleanup target missing")
	}
	for _, extra := range []string{"&InstanceId.2=i-87654321", "&Action=RunInstances", "&DryRun=true"} {
		r, _ := http.NewRequest("POST", "https://ec2.eu-west-1.amazonaws.com/", strings.NewReader(body+extra))
		if TerminationTarget(r) != "" {
			t.Fatal("broad cleanup exemption")
		}
	}
}
func TestAWSCredentialAndErrorIsolation(t *testing.T) {
	for _, s := range []string{strings.Replace(fixtureCredential, "eu-west-1", "evil.amazonaws.com/", 1), fixtureCredential + `{}`, strings.Replace(fixtureCredential, "\"region\"", "\"endpoint\"", 1)} {
		_, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "a", HTTPClient: &http.Client{}, Credentials: testCredential(s)})
		if e == nil || strings.Contains(e.Error(), "fixture-secret") {
			t.Fatal(e)
		}
	}
	e := classify("create", 400, []byte(`<Response><Errors><Error><Code>VcpuLimitExceeded</Code><Message>fixture-secret</Message></Error></Errors></Response>`))
	if e.Class != providers.ErrorCapacity || strings.Contains(e.Error(), "fixture-secret") {
		t.Fatal(e)
	}
}
func TestAWSSigningDeterministicAndSensitiveToBody(t *testing.T) {
	c, _ := credentials(context.Background(), testCredential(fixtureCredential))
	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	req, _ := http.NewRequest("POST", "https://ec2.eu-west-1.amazonaws.com/", nil)
	sign(req, "Action=RunInstances", c, "ec2", now)
	first := req.Header.Get("Authorization")
	sign(req, "Action=RunInstances", c, "ec2", now)
	if first != req.Header.Get("Authorization") {
		t.Fatal("unstable signature")
	}
	sign(req, "Action=TerminateInstances", c, "ec2", now)
	if first == req.Header.Get("Authorization") {
		t.Fatal("body not authenticated")
	}
}
func TestAWSExpectedAccountScopeRejectsCredentialRebind(t *testing.T) {
	calls := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return response("GetCallerIdentity", fixtureRead("GetCallerIdentity")), nil
	})
	d.client.expected = "999999999999:eu-west-1"
	if _, e := d.ListServers(context.Background()); !providers.IsClass(e, providers.ErrorAuthentication) || calls != 1 {
		t.Fatal("wrong account inventory accepted", e, calls)
	}
}
func TestAWSAbsenceWithoutEvidenceIsInconclusive(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		return response("DescribeInstances", `<reservationSet/>`), nil
	})
	if _, e := d.GetServer(context.Background(), "eu-west-1:i-12345678"); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal("transient absence treated as deleted", e)
	}
}

// Published AWS CRT signing vectors (test credentials, never production secrets):
// https://github.com/awslabs/aws-c-auth/tree/main/tests/aws-signing-test-suite/v4
func TestAWSSignatureOfficialVectors(t *testing.T) {
	for _, x := range []struct{ token, signature string }{{"", "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"}, {"6e86291e8372ff2a2260956d9b8aae1d763fbf315fa00fa31553b73ebf194267", "07ec1639c89043aa0e3e2de82b96708f198cceab042d4a97044c66dd9f74e7f8"}} {
		c := credential{AccessKey: "AKIDEXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", Region: "us-east-1", SessionToken: x.token}
		r, _ := http.NewRequest("GET", "https://example.amazonaws.com/", nil)
		sign(r, "", c, "service", time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC))
		if !strings.HasSuffix(r.Header.Get("Authorization"), "Signature="+x.signature) {
			t.Fatal("official signing vector mismatch")
		}
	}
}
