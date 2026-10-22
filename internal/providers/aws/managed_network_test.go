package aws

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type networkJournal struct {
	mu     sync.Mutex
	record providers.AccountResource
	exists bool
	stable bool
}

func (j *networkJournal) WithLock(ctx context.Context, scope, name string, fn func(providers.AccountResourceSession) error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.exists {
		j.record = providers.AccountResource{Scope: scope, Name: name, Nonce: "durable-fixture-nonce"}
		j.exists = true
	}
	return fn(j)
}
func (j *networkJournal) List(context.Context) ([]providers.AccountResource, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.exists || j.record.Complete {
		return []providers.AccountResource{}, nil
	}
	return []providers.AccountResource{j.record}, nil
}
func (j *networkJournal) Record() providers.AccountResource { return j.record }
func (j *networkJournal) BeginAttempt(context.Context) (bool, error) {
	if j.record.Attempted {
		return false, nil
	}
	j.record.Attempted = true
	return true, nil
}
func (j *networkJournal) Bind(_ context.Context, id string) error {
	if j.record.ID != "" && j.record.ID != id {
		return errors.New("rebind")
	}
	j.record.ID = id
	return nil
}
func (j *networkJournal) Missing(context.Context) (bool, error) { return j.stable, nil }
func (j *networkJournal) Finish(context.Context) error          { j.record.Complete = true; return nil }
func groupXML(name, nonce string, ready bool) string {
	permissions := ""
	if ready {
		for _, protocol := range []string{"tcp", "udp"} {
			permissions += `<item><ipProtocol>` + protocol + `</ipProtocol><fromPort>1</fromPort><toPort>65535</toPort><ipRanges><item><cidrIp>0.0.0.0/0</cidrIp></item></ipRanges></item>`
		}
	}
	return `<securityGroupInfo><item><groupId>sg-12345678</groupId><groupName>` + name + `</groupName><vpcId>vpc-12345678</vpcId><tagSet><item><key>dob:managed</key><value>true</value></item><item><key>dob:account</key><value>account-a</value></item><item><key>dob:purpose</key><value>account-network</value></item><item><key>dob:intent</key><value>` + nonce + `</value></item></tagSet><ipPermissions>` + permissions + `</ipPermissions></item></securityGroupInfo>`
}
func TestAWSManagedNetworkUnknownOutcomeAndOwnership(t *testing.T) {
	for _, mode := range []string{"lost_create", "lost_rules", "foreign", "attached"} {
		t.Run(mode, func(t *testing.T) {
			j := &networkJournal{}
			created, ready := false, false
			creates, grants, deletes := 0, 0, 0
			var d *Driver
			d = openTest(t, func(r *http.Request) (*http.Response, error) {
				q := query(r)
				a := q.Get("Action")
				switch a {
				case "DescribeSecurityGroups":
					if !created {
						return response(a, "<securityGroupInfo/>"), nil
					}
					nonce := j.record.Nonce
					if mode == "foreign" {
						nonce = "foreign-nonce"
					}
					return response(a, groupXML(d.networkName("vpc-12345678"), nonce, ready)), nil
				case "CreateSecurityGroup":
					creates++
					if !j.record.Attempted || q.Get("TagSpecification.1.Tag.4.Value") != j.record.Nonce {
						t.Fatal("mutation before intent")
					}
					created = true
					if mode == "lost_create" && creates == 1 {
						return nil, errors.New("lost response")
					}
					return response(a, "<groupId>sg-12345678</groupId>"), nil
				case "AuthorizeSecurityGroupIngress":
					grants++
					if j.record.ID != "sg-12345678" {
						t.Fatal("rules before binding")
					}
					ready = true
					if mode == "lost_rules" && grants == 1 {
						return nil, errors.New("lost response")
					}
					return response(a, "<return>true</return>"), nil
				case "DescribeNetworkInterfaces":
					if mode == "attached" {
						return response(a, "<networkInterfaceSet><item><networkInterfaceId>eni-12345678</networkInterfaceId></item></networkInterfaceSet>"), nil
					}
					return response(a, "<networkInterfaceSet/>"), nil
				case "DeleteSecurityGroup":
					deletes++
					created = false
					return response(a, "<return>true</return>"), nil
				}
				return response(a, networkFixture(a)), nil
			})
			d.resources = j
			cred, _ := credentials(context.Background(), d.client.credentials)
			cred.ManagedNetwork = true
			cred.Groups = nil
			d.client.credentials = cloudutil.NewJSONCredential(cred)
			_, e := d.ensureNetwork(context.Background(), cred)
			if mode == "foreign" {
				if e == nil || grants != 0 {
					t.Fatal("foreign network modified", e)
				}
				return
			}
			if mode != "attached" && e == nil {
				t.Fatal("lost response treated as confirmed")
			}
			id, e := d.ensureNetwork(context.Background(), cred)
			if e != nil || id != "sg-12345678" || creates != 1 || grants != 1 {
				t.Fatal("recovery duplicated mutation", id, e, creates, grants)
			}
			e = d.CleanupAccountResources(context.Background())
			if e == nil {
				t.Fatal("cleanup accepted premature absence")
			}
			if mode == "attached" {
				if deletes != 0 {
					t.Fatal("attached group deleted")
				}
				return
			}
			if deletes != 1 {
				t.Fatal(deletes)
			}
			j.stable = true
			if e = d.CleanupAccountResources(context.Background()); e != nil || !j.record.Complete {
				t.Fatal(e)
			}
		})
	}
}
func TestAWSManagedNetworkConcurrentIntent(t *testing.T) {
	j := &networkJournal{}
	exists, ready := false, false
	creates, grants := 0, 0
	var d *Driver
	d = openTest(t, func(r *http.Request) (*http.Response, error) {
		q := query(r)
		a := q.Get("Action")
		switch a {
		case "DescribeSecurityGroups":
			if exists {
				return response(a, groupXML(d.networkName("vpc-12345678"), j.record.Nonce, ready)), nil
			}
			return response(a, "<securityGroupInfo/>"), nil
		case "CreateSecurityGroup":
			creates++
			exists = true
			return response(a, "<groupId>sg-12345678</groupId>"), nil
		case "AuthorizeSecurityGroupIngress":
			grants++
			ready = true
			return response(a, "<return>true</return>"), nil
		}
		return response(a, networkFixture(a)), nil
	})
	d.resources = j
	c, _ := credentials(context.Background(), d.client.credentials)
	c.ManagedNetwork = true
	c.Groups = nil
	d.client.credentials = cloudutil.NewJSONCredential(c)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := d.ensureNetwork(context.Background(), c); results <- e }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	if creates != 1 || grants != 1 {
		t.Fatal(creates, grants)
	}
}
func TestAWSNetworkDeletionTargetStrict(t *testing.T) {
	for _, body := range []string{"Action=DeleteSecurityGroup&GroupId=sg-12345678&Version=2016-11-15", "Action=DeleteSecurityGroup&GroupId=sg-12345678&GroupId=sg-87654321&Version=2016-11-15", "Action=DeleteSecurityGroup&GroupId=i-12345678&Version=2016-11-15"} {
		r, _ := http.NewRequest("POST", "https://ec2.eu-west-1.amazonaws.com/", strings.NewReader(body))
		target := NetworkDeletionTarget(r)
		if (body == "Action=DeleteSecurityGroup&GroupId=sg-12345678&Version=2016-11-15") != (target == "eu-west-1:sg-12345678") {
			t.Fatal(target)
		}
	}
}

func TestAWSNetworkReadFailureNeverDispatchesCompute(t *testing.T) {
	posts := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			posts++
		}
		return nil, errors.New("connection unavailable")
	})
	out, e := d.CreateServer(context.Background(), providers.CreateServerRequest{Name: "fixture", RegionID: "eu-west-1", PlanID: "t3.micro", ImageID: "ami-12345678", Identity: "fixture", SSHAuthorizedKeys: []string{publicKey(t)}})
	if e == nil || posts != 0 || out.Outcome != providers.OutcomeRejected || !out.ComputeNotDispatched {
		t.Fatal("pre-compute failure became unknown create", out, e, posts)
	}
}
