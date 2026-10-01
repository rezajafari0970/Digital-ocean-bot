package droplets

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type computeStub struct {
	creates, deletes     int
	createErr, deleteErr error
	createResult         *providers.CreateServerResult
	servers              []providers.Server
}

func (p *computeStub) CreateServer(context.Context, providers.CreateServerRequest) (providers.CreateServerResult, error) {
	p.creates++
	if p.createErr != nil {
		return providers.CreateServerResult{}, p.createErr
	}
	if p.createResult != nil {
		return *p.createResult, nil
	}
	return providers.CreateServerResult{ServerID: "42", Outcome: providers.OutcomeAccepted}, nil
}
func (p *computeStub) GetServer(context.Context, string) (providers.Server, error) {
	return providers.Server{}, nil
}
func (p *computeStub) ListServers(context.Context) ([]providers.Server, error) { return p.servers, nil }
func (p *computeStub) DeleteServer(context.Context, string) error              { p.deletes++; return p.deleteErr }
func (p *computeStub) FindServerByIdentity(_ context.Context, id string) ([]providers.Server, error) {
	out := []providers.Server{}
	for _, s := range p.servers {
		for _, t := range s.Tags {
			if t == id {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}
func taggedServer(id, name, region string, tags ...string) providers.Server {
	return providers.Server{ID: id, Name: name, RegionID: region, Tags: tags}
}
