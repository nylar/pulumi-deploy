package aws

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/kms"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ECS struct {
	Name          string
	EnableLogging bool

	Out struct {
		Cluster      *ecs.Cluster
		TaskExecRole *iam.Role
	}
}

func (e *ECS) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("missing ECS.Name")
	}

	return nil
}

func (e *ECS) Run(ctx *pulumi.Context) error {
	if err := e.Validate(); err != nil {
		return err
	}
	var cluster *ecs.Cluster
	if e.EnableLogging {
		logKey, err := kms.NewKey(ctx, fmt.Sprintf("%v-log-key", e.Name), &kms.KeyArgs{
			Description:          pulumi.String(fmt.Sprintf("%v KMS encryption key for logging container activity", e.Name)),
			DeletionWindowInDays: pulumi.Int(7),
		})
		if err != nil {
			return err
		}
		ctx.Export("CLUSTER-LOG-KMS-KEY-ID", logKey.ID())

		logGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("%v-log-group", e.Name), nil)
		if err != nil {
			return err
		}
		ctx.Export("CLUSTER-LOG-GROUP-ID", logGroup.ID())

		cluster, err = ecs.NewCluster(ctx, e.Name, &ecs.ClusterArgs{
			Configuration: &ecs.ClusterConfigurationArgs{
				ExecuteCommandConfiguration: &ecs.ClusterConfigurationExecuteCommandConfigurationArgs{
					KmsKeyId: logKey.Arn,
					Logging:  pulumi.String("OVERRIDE"),
					LogConfiguration: &ecs.ClusterConfigurationExecuteCommandConfigurationLogConfigurationArgs{
						CloudWatchEncryptionEnabled: pulumi.Bool(true),
						CloudWatchLogGroupName:      logGroup.Name,
					},
				},
			},
		})
		if err != nil {
			return err
		}
	} else {
		var err error
		cluster, err = ecs.NewCluster(ctx, e.Name, &ecs.ClusterArgs{
			Settings: ecs.ClusterSettingArray{
				&ecs.ClusterSettingArgs{
					Name:  pulumi.String("containerInsights"),
					Value: pulumi.String("enabled"),
				},
			},
		})
		if err != nil {
			return err
		}
	}
	e.Out.Cluster = cluster
	ctx.Export("CLUSTER-ID", cluster.ID())

	// Create IAM role that can be used by our service's task.
	taskExecRole, err := iam.NewRole(ctx, fmt.Sprintf("%v-task-exec-role", e.Name), &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(
			`{
				"Version": "2008-10-17",
				"Statement": [{
					"Sid": "",
					"Effect": "Allow",
					"Principal": {
						"Service": "ecs-tasks.amazonaws.com"
					},
					"Action": "sts:AssumeRole"
				}]
			}`),
	})
	if err != nil {
		return err
	}
	e.Out.TaskExecRole = taskExecRole

	_, err = iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("%v-task-exec-policy", e.Name), &iam.RolePolicyAttachmentArgs{
		Role:      taskExecRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
	})
	if err != nil {
		return err
	}

	return nil
}
