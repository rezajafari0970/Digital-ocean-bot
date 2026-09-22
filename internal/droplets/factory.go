package droplets

import "github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"

func BuildCreateOperation(accountID string, profile Profile) jobs.Operation {
	return jobs.Operation{
		AccountID:      accountID,
		Kind:           "CREATE_DROPLET",
		IdempotencyKey: "create:" + accountID + ":" + profile.Name,
		State:          jobs.OperationPlanned,
	}
}
