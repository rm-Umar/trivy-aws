package sns

import (
	"github.com/aquasecurity/trivy-aws/internal/adapters/cloud/aws"
	"github.com/aquasecurity/trivy/pkg/iac/providers/aws/sns"
	"github.com/aquasecurity/trivy/pkg/iac/state"
	"github.com/aquasecurity/trivy/pkg/iac/types"
	snsapi "github.com/aws/aws-sdk-go-v2/service/sns"
	snsTypes "github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/aquasecurity/trivy-aws/pkg/concurrency"
)

type adapter struct {
	*aws.RootAdapter
	client *snsapi.Client
}

func init() {
	aws.RegisterServiceAdapter(&adapter{})
}

func (a *adapter) Provider() string {
	return "aws"
}

func (a *adapter) Name() string {
	return "sns"
}

func (a *adapter) Adapt(root *aws.RootAdapter, state *state.State) error {

	a.RootAdapter = root
	a.client = snsapi.NewFromConfig(root.SessionConfig())
	var err error

	state.AWS.SNS.Topics, err = a.getTopics()
	if err != nil {
		return err
	}

	return nil
}

func (a *adapter) getTopics() (queues []sns.Topic, err error) {

	a.Tracker().SetServiceLabel("Discovering SNS topics...")
	var apiTopics []snsTypes.Topic
	var input snsapi.ListTopicsInput

	for {
		output, err := a.client.ListTopics(a.Context(), &input)
		if err != nil {
			return nil, err
		}
		apiTopics = append(apiTopics, output.Topics...)
		a.Tracker().SetTotalResources(len(apiTopics))
		if output.NextToken == nil {
			break
		}
		input.NextToken = output.NextToken
	}

	a.Tracker().SetServiceLabel("Adapting SNS topics...")
	return concurrency.Adapt(apiTopics, a.RootAdapter, a.adaptTopic), nil

}

func (a *adapter) adaptTopic(topic snsTypes.Topic) (*sns.Topic, error) {

	topicMetadata := a.CreateMetadataFromARN(*topic.TopicArn)

	t := sns.NewTopic(*topic.TopicArn, topicMetadata)
	topicAttributes, err := a.client.GetTopicAttributes(a.Context(), &snsapi.GetTopicAttributesInput{
		TopicArn: topic.TopicArn,
	})
	if err != nil {
		a.Debug("Failed to get topic attributes for '%s': %s", *topic.TopicArn, err)
		return nil, err
	}

	if kmsKeyID, ok := topicAttributes.Attributes["KmsMasterKeyId"]; ok {
		t.Encryption.KMSKeyID = types.String(kmsKeyID, topicMetadata)
	}

	return t, nil

}
