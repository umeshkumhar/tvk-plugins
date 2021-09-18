package targetbrowser

import (
	"github.com/google/go-querystring/query"
	"github.com/trilioData/tvk-plugins/internal"
)

// TrilioResourcesListOptions for trilio-resources
type TrilioResourcesListOptions struct {
	BackupUID     string   `url:"backupUID"`
	BackupPlanUID string   `url:"backupPlanUID"`
	Kinds         []string `url:"kinds"`
	CommonListOptions
}

// GetTrilioResources returns trilio resources of particular backup on mounted target
func (auth *AuthInfo) GetTrilioResources(options *TrilioResourcesListOptions, backupUIDs []string) error {
	values, err := query.Values(options)
	if err != nil {
		return err
	}
	queryParam := values.Encode()
	resp, apiErr := auth.TriggerAPIs(queryParam, internal.TrilioResourcesAPIPath, backupUIDs)
	if apiErr != nil {
		return apiErr
	}

	if options.OutputFormat == "" || options.OutputFormat == internal.FormatWIDE {
		options.OutputFormat = internal.FormatYAML
	}

	return PrintFormattedResponse(internal.TrilioResourcesAPIPath, string(resp), options.OutputFormat)
}
