package msk

import (
	"github.com/aquasecurity/defsec/pkg/providers/aws/msk"
	"github.com/aquasecurity/defsec/pkg/state"
	defsecTypes "github.com/aquasecurity/defsec/pkg/types"
	api "github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"

	"github.com/aquasecurity/trivy-aws/internal/adapters"
	"github.com/aquasecurity/trivy-aws/pkg/concurrency"
)

type adapter struct {
	*adapters.RootAdapter
	api *api.Client
}

func init() {
	adapters.RegisterServiceAdapter(&adapter{})
}

func (a *adapter) Provider() string {
	return "aws"
}

func (a *adapter) Name() string {
	return "msk"
}

func (a *adapter) Adapt(root *adapters.RootAdapter, state *state.State) error {

	a.RootAdapter = root
	a.api = api.NewFromConfig(root.SessionConfig())
	var err error

	state.AWS.MSK.Clusters, err = a.getClusters()
	if err != nil {
		return err
	}

	return nil
}

func (a *adapter) getClusters() ([]msk.Cluster, error) {

	a.Tracker().SetServiceLabel("Discovering clusters...")

	var apiClusters []types.ClusterInfo
	var input api.ListClustersInput
	for {
		output, err := a.api.ListClusters(a.Context(), &input)
		if err != nil {
			return nil, err
		}
		apiClusters = append(apiClusters, output.ClusterInfoList...)
		a.Tracker().SetTotalResources(len(apiClusters))
		if output.NextToken == nil {
			break
		}
		input.NextToken = output.NextToken
	}

	a.Tracker().SetServiceLabel("Adapting clusters...")
	return concurrency.Adapt(apiClusters, a.RootAdapter, a.adaptCluster), nil
}

func (a *adapter) adaptCluster(apiCluster types.ClusterInfo) (*msk.Cluster, error) {

	metadata := a.CreateMetadataFromARN(*apiCluster.ClusterArn)

	var encInTransitClientBroker, encAtRestKMSKeyId string
	var encAtRestEnabled bool
	if apiCluster.EncryptionInfo != nil {
		if apiCluster.EncryptionInfo.EncryptionInTransit != nil {
			encInTransitClientBroker = string(apiCluster.EncryptionInfo.EncryptionInTransit.ClientBroker)
		}

		if apiCluster.EncryptionInfo.EncryptionAtRest != nil {
			encAtRestKMSKeyId = *apiCluster.EncryptionInfo.EncryptionAtRest.DataVolumeKMSKeyId
			encAtRestEnabled = true
		}
	}

	var logS3, logCW, logFH bool
	if apiCluster.LoggingInfo != nil && apiCluster.LoggingInfo.BrokerLogs != nil {
		logs := apiCluster.LoggingInfo.BrokerLogs
		if logs.S3 != nil {
			logS3 = logs.S3.Enabled
		}
		if logs.CloudWatchLogs != nil {
			logCW = logs.CloudWatchLogs.Enabled
		}
		if logs.Firehose != nil {
			logFH = logs.Firehose.Enabled
		}
	}

	return &msk.Cluster{
		Metadata: metadata,
		EncryptionInTransit: msk.EncryptionInTransit{
			Metadata:     metadata,
			ClientBroker: defsecTypes.String(encInTransitClientBroker, metadata),
		},
		EncryptionAtRest: msk.EncryptionAtRest{
			Metadata:  metadata,
			KMSKeyARN: defsecTypes.String(encAtRestKMSKeyId, metadata),
			Enabled:   defsecTypes.Bool(encAtRestEnabled, metadata),
		},
		Logging: msk.Logging{
			Metadata: metadata,
			Broker: msk.BrokerLogging{
				Metadata: metadata,
				S3: msk.S3Logging{
					Metadata: metadata,
					Enabled:  defsecTypes.Bool(logS3, metadata),
				},
				Cloudwatch: msk.CloudwatchLogging{
					Metadata: metadata,
					Enabled:  defsecTypes.Bool(logCW, metadata),
				},
				Firehose: msk.FirehoseLogging{
					Metadata: metadata,
					Enabled:  defsecTypes.Bool(logFH, metadata),
				},
			},
		},
	}, nil
}
