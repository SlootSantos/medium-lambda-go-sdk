package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const (
	bucketNameOrigin     = "medium-lambda-go-sdk-origin-white-1"
	bucketNameSourceCode = "medium-lambda-go-sdk-source-code-white-1"
	bucketACLPublic      = "public-read"
	iamRoleName          = "medium-lambda-go-sdk-role-white-11"
	lambdaFunctionName   = "medium-lambda-go-sdk-function-white-11"
)

func main() {
	log.Println("Hello Lambda")

	awsSess := initAWSSession()
	createS3Buckets(awsSess)
	uploadZip(awsSess)
	roleARN := createIAMRole(awsSess)
	time.Sleep(time.Second * 20)
	functionARN := createLambdaFunction(awsSess, roleARN)
	createCloudfrontDistro(awsSess, functionARN)
}

func initAWSSession() *session.Session {
	s, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	})
	if err != nil {
		panic("Did you pass the credentials?")
	}

	return s
}

func createS3Buckets(s *session.Session) {
	s3Handler := s3.New(s)

	createBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucketNameOrigin),
		ACL:    aws.String(bucketACLPublic),
	}

	_, err := s3Handler.CreateBucket(createBucketInput)
	if err != nil {
		panic("Could not create origin bucket" + err.Error())
	}

	createSourceCodeBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucketNameSourceCode),
		ACL:    aws.String(bucketACLPublic),
	}

	_, err = s3Handler.CreateBucket(createSourceCodeBucketInput)
	if err != nil {
		panic("Could not create source code bucket" + err.Error())
	}
}

func uploadZip(s *session.Session) {
	file, err := os.Open("source.zip")
	if err != nil {
		panic("Could not load source zip" + err.Error())
	}

	defer file.Close()

	uploader := s3manager.NewUploader(s)

	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucketNameSourceCode),
		Key:    aws.String(filepath.Base("source.zip")),
		Body:   file,
	})
	if err != nil {
		panic("Could not upload source zip" + err.Error())
	}
}

func createIAMRole(s *session.Session) string {
	iamHandler := iam.New(s)
	role, err := iamHandler.CreateRole(&iam.CreateRoleInput{
		Path:                     aws.String("/service-role/"),
		AssumeRolePolicyDocument: aws.String("{\"Version\": \"2012-10-17\",\"Statement\": [{\"Effect\": \"Allow\",\"Principal\": {\"Service\": [\"lambda.amazonaws.com\",\"edgelambda.amazonaws.com\"]},\"Action\": \"sts:AssumeRole\"}]}"),
		RoleName:                 aws.String(iamRoleName),
	})
	if err != nil {
		panic("Could not create IAM role" + err.Error())
	}

	_, err = iamHandler.PutRolePolicy(&iam.PutRolePolicyInput{
		PolicyName:     aws.String("stackers-lambda-exec-policy"),
		PolicyDocument: aws.String("{\"Version\": \"2012-10-17\", \"Statement\": [ { \"Effect\": \"Allow\", \"Action\": [ \"logs:CreateLogGroup\", \"logs:CreateLogStream\", \"logs:PutLogEvents\" ], \"Resource\": [ \"arn:aws:logs:*:*:*\" ] } ] }"),
		RoleName:       aws.String(iamRoleName),
	})
	if err != nil {
		panic("Cloud not put role policy" + err.Error())
	}

	return *role.Role.Arn
}

func createLambdaFunction(s *session.Session, roleARN string) string {
	lambdaHandler := lambda.New(s)
	res, err := lambdaHandler.CreateFunction(&lambda.CreateFunctionInput{
		FunctionName: aws.String(lambdaFunctionName),
		Handler:      aws.String("index.handler"),
		Runtime:      aws.String("nodejs12.x"),
		Role:         aws.String(roleARN),
		Code: &lambda.FunctionCode{
			S3Bucket: aws.String(bucketNameSourceCode),
			S3Key:    aws.String("source.zip"),
		},
	})
	if err != nil {
		panic("Could not create lambda function" + err.Error())
	}

	_, err = lambdaHandler.PublishVersion(&lambda.PublishVersionInput{
		FunctionName: aws.String(*res.FunctionArn),
		Description:  aws.String("Example function for Medium"),
	})
	if err != nil {
		panic("Could not publish function Version" + err.Error())
	}

	return *res.FunctionArn + ":1"
}

func createCloudfrontDistro(s *session.Session, functionARN string) {
	cdnHandler := cloudfront.New(s)
	config := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(bucketNameOrigin),
			Comment:         aws.String(bucketNameOrigin),
			Enabled:         aws.Bool(true),
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(1),
				Items: []*cloudfront.Origin{
					{
						S3OriginConfig: &cloudfront.S3OriginConfig{
							OriginAccessIdentity: aws.String(""),
						},
						Id:         aws.String("ORIGIN_ID"),
						DomainName: aws.String(bucketNameOrigin + ".s3.amazonaws.com"),
					},
				},
			},
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				MinTTL:               aws.Int64(10),
				Compress:             aws.Bool(true),
				TargetOriginId:       aws.String("ORIGIN_ID"),
				ViewerProtocolPolicy: aws.String("redirect-to-https"),
				LambdaFunctionAssociations: &cloudfront.LambdaFunctionAssociations{
					Items: []*cloudfront.LambdaFunctionAssociation{
						{
							LambdaFunctionARN: aws.String(functionARN),
							IncludeBody:       aws.Bool(false),
							EventType:         aws.String("origin-request"),
						},
					},
					Quantity: aws.Int64(1),
				},
				ForwardedValues: &cloudfront.ForwardedValues{
					QueryString: aws.Bool(false),
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("none"),
					},
				},
				TrustedSigners: &cloudfront.TrustedSigners{
					Quantity: aws.Int64(0),
					Enabled:  aws.Bool(false),
				},
			},
		},
	}

	_, err := cdnHandler.CreateDistribution(config)
	if err != nil {
		panic("Could not create CDN" + err.Error())
	}
}
