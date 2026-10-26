package aws

import (
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
)

// Only fixed known values cross the diagnostic boundary. AWS messages can echo
// access keys, request values and encoded authorization details; never retain them.
type wireError struct{ code, format string }

func (e *wireError) Error() string { return "AWS " + e.code }

var wireCodes = wordSet(`AuthFailure InvalidClientTokenId InvalidAccessKeyId SignatureDoesNotMatch InvalidSignatureException IncompleteSignature MissingAuthenticationToken ExpiredToken ExpiredTokenException RequestExpired UnauthorizedOperation AccessDenied AccessDeniedException OperationNotPermitted RequestLimitExceeded Throttling ThrottlingException InstanceLimitExceeded VcpuLimitExceeded InsufficientInstanceCapacity InsufficientHostCapacity Unsupported InvalidAMIID.NotFound InvalidAMIID.Unavailable InvalidInstanceID.NotFound IdempotentParameterMismatch OptInRequired PendingVerification Blocked AccountDisabled InvalidParameter InvalidParameterValue InvalidParameterCombination InvalidParameterDependency InvalidQueryParameter MalformedQueryString MissingAction MissingParameter UnknownParameter InvalidAction ValidationError UnsupportedOperation UnsupportedProtocol InvalidPaginationToken InvalidNextToken RequestTimeout InternalError InternalFailure ServiceUnavailable NON_AWS_ERROR_RESPONSE INVALID_ERROR_RESPONSE`)
var localCodes = wordSet(`REGIONAL_NETWORK_LIMIT INVALID_REGIONAL_NETWORK DUPLICATE_REGION_SELECTION REGIONAL_PLAN_MISMATCH SUBNET_ZONE_UNAVAILABLE NO_SUPPORTED_INSTANCE_TYPES NO_UBUNTU_IMAGES NO_SUPPORTED_UBUNTU_IMAGES INVALID_CREDENTIAL_JSON TEMPORARY_SESSION_TOKEN_REQUIRED REGION_SCOPE_CHANGED ACCOUNT_SCOPE_MISMATCH INVALID_REGION_LIST REGIONS_MISSING REGION_NOT_ENABLED NETWORK_SELECTION_REQUIRED INVALID_SECURITY_GROUPS SUBNET_NOT_PUBLIC_OR_ACCESSIBLE SUBNET_UNAVAILABLE PUBLIC_SUBNET_UNAVAILABLE SECURITY_GROUP_VPC_MISMATCH INCOMPLETE_SECURITY_GROUPS INVALID_RESPONSE INVALID_RESPONSE_ENVELOPE INVALID_RESPONSE_BODY INVALID_ERROR_RESPONSE MISSING_COLLECTION INVALID_COLLECTION DUPLICATE_SET INVALID_COLLECTION_IDENTITY INVALID_PAGE PAGINATION_LOOP PAGINATION_LIMIT TRANSPORT_ERROR INVALID_ACCOUNT_IDENTITY INVALID_SUBNET INVALID_SUBNET_ID SUBNET_ID_MISMATCH REGION_MISMATCH EMPTY_RESERVATION INVALID_INSTANCE INVALID_REQUEST REQUEST_FAILED REQUEST_REJECTED AUTHENTICATION_FAILED PERMISSION_DENIED ACCOUNT_ACCESS_REQUIRED ACCOUNT_LOCKED PROVIDER_UNAVAILABLE RATE_LIMITED`)
var operations = wordSet(`DescribeRegions GetCallerIdentity DescribeSubnets DescribeRouteTables DescribeNetworkAcls DescribeInternetGateways DescribeSecurityGroups DescribeNetworkInterfaces DescribeInstances DescribeImages DescribeInstanceTypes DescribeInstanceTypeOfferings RunInstances TerminateInstances CreateSecurityGroup DeleteSecurityGroup AuthorizeSecurityGroupIngress credential discovery account catalog inventory network server get_server create_server delete_server find_server`)

func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, v := range strings.Fields(s) {
		m[v] = true
	}
	return m
}
func wireProblem(e *providers.Error, code, format string) *providers.Error {
	if !wireCodes[code] {
		code = "UNRECOGNIZED_AWS_ERROR"
	}
	e.Cause = &wireError{code: code, format: format}
	operation := "unknown"
	if operations[e.Operation] {
		operation = e.Operation
	}
	// Background observation persists Error(), not Diagnostic(). Keep its
	// original allowlisted AWS cause instead of only AUTHENTICATION_FAILED.
	e.Message += "; code=" + code + "; operation=" + operation
	return e
}

type DiagnosticInfo struct {
	Code, Operation, ResponseFormat, Guidance, RequiredPermission string
	IdentityVerified                                              bool
}

// Only STS success followed by a genuine EC2 AuthFailure constructs this error.
// Preserve the wire cause without classifying a proven key as invalid.
type identityVerifiedEC2Error struct{ cause error }

func (e *identityVerifiedEC2Error) Error() string {
	return "AWS identity verified; EC2 service access could not be validated"
}
func (e *identityVerifiedEC2Error) Unwrap() error {
	var pe *providers.Error
	status := 0
	if errors.As(e.cause, &pe) {
		status = pe.StatusCode
	}
	return &providers.Error{StatusCode: status, Class: providers.ErrorPermissionDenied, Operation: "DescribeRegions", Code: "ACCOUNT_ACCESS_REQUIRED", Message: e.Error(), Cause: e.cause}
}

func Diagnostic(err error) DiagnosticInfo {
	d := DiagnosticInfo{Code: "UNKNOWN", Operation: "unknown", ResponseFormat: "local", Guidance: "Retry validation and use this reference to inspect the failed step."}
	var pe *providers.Error
	if errors.As(err, &pe) {
		if operations[pe.Operation] {
			d.Operation = pe.Operation
		}
		if localCodes[pe.Code] {
			d.Code = pe.Code
		}
	}
	var we *wireError
	if errors.As(err, &we) {
		d.Code = we.code
		d.ResponseFormat = we.format
	}
	if strings.HasPrefix(d.Operation, "Describe") || d.Operation == "RunInstances" || d.Operation == "TerminateInstances" || d.Operation == "CreateSecurityGroup" || d.Operation == "DeleteSecurityGroup" || d.Operation == "AuthorizeSecurityGroupIngress" {
		d.RequiredPermission = "ec2:" + d.Operation
	}
	switch d.Code {
	case "DUPLICATE_REGION_SELECTION":
		d.Guidance = "Each additional region must be different from the primary region and other selected regions."
	case "SUBNET_ZONE_UNAVAILABLE":
		d.Guidance = "AWS did not return a valid availability zone for the selected subnet. Reload network choices and select a verified public subnet."
	case "INVALID_REGIONAL_NETWORK", "REGIONAL_NETWORK_LIMIT":
		d.Guidance = "Select a primary region and at most four distinct additional regions, each with its own verified public subnet."
	case "NO_SUPPORTED_INSTANCE_TYPES":
		d.RequiredPermission = ""
		d.Guidance = "AWS returned no supported x86_64 on-demand instance type compatible with the supported Ubuntu images in the selected region and subnet availability zone. Choose another verified public subnet or enabled region."
	case "NO_UBUNTU_IMAGES":
		d.RequiredPermission = ""
		d.Guidance = "AWS returned no Canonical Ubuntu server images in the selected region. Check this region and any Allowed AMIs account/organization policy, then reload provider options."
	case "NO_SUPPORTED_UBUNTU_IMAGES":
		d.RequiredPermission = ""
		d.Guidance = "AWS returned images in the selected region, but none passed the supported Ubuntu image checks. Use this reference to inspect the driver image filter; changing credentials is not established as the remedy."

	case "AuthFailure", "InvalidClientTokenId", "InvalidAccessKeyId", "INVALID_CREDENTIAL_JSON":
		d.Guidance = "Check the access key ID and matching secret key. Temporary credentials must include the matching session token."
	case "ExpiredToken", "ExpiredTokenException", "TEMPORARY_SESSION_TOKEN_REQUIRED":
		d.Guidance = "Enter a complete, current temporary credential set: access key ID, secret access key and session token."
	case "RequestExpired":
		d.Guidance = "Check the controller clock and credential expiry; the signed request was outside AWS's accepted time window."
	case "SignatureDoesNotMatch", "IncompleteSignature", "InvalidSignatureException", "MissingAuthenticationToken":
		d.Guidance = "AWS rejected the request authentication/signature. Check the matching key pair and session token; inspect signing and the selected connection using this reference."
	case "UnauthorizedOperation", "AccessDenied", "AccessDeniedException", "OperationNotPermitted":
		d.Guidance = "The AWS identity or an account/organization policy denies this operation. Check the named permission; a new server has not been created by validation."
	case "Blocked", "AccountDisabled":
		d.Guidance = "AWS reports this account blocked or disabled. Resolve the account restriction in AWS before retrying."
	case "OptInRequired", "PendingVerification":
		d.Guidance = "Complete AWS account/service activation or verification, and confirm access to the selected region."
	case "REGION_NOT_ENABLED":
		d.Guidance = "Choose an enabled commercial AWS region in Advanced overrides."
	case "NETWORK_SELECTION_REQUIRED", "SUBNET_NOT_PUBLIC_OR_ACCESSIBLE", "PUBLIC_SUBNET_UNAVAILABLE", "SUBNET_UNAVAILABLE":
		d.Guidance = "Select a reachable public subnet in the chosen region, then validate its network settings again."
	case "NON_AWS_ERROR_RESPONSE", "INVALID_ERROR_RESPONSE":
		d.Guidance = "The selected connection returned an invalid AWS error response. Check the proxy/connection; this response does not establish that the credentials are wrong."
	case "TRANSPORT_ERROR":
		d.Guidance = "The selected connection could not complete the AWS request. Check its proxy and connectivity."
	case "RequestLimitExceeded", "Throttling", "ThrottlingException":
		d.Guidance = "AWS throttled this request. Retry after the existing backoff."
	case "InvalidParameter", "InvalidParameterValue", "InvalidParameterCombination", "InvalidParameterDependency", "InvalidQueryParameter", "MalformedQueryString", "MissingAction", "MissingParameter", "UnknownParameter", "InvalidAction", "ValidationError", "UnsupportedOperation", "UnsupportedProtocol":
		d.Guidance = "AWS rejected a parameter or operation. The operation and exact error code below identify the driver request to inspect; changing credentials is not established as the remedy."
	}
	// A named EC2 operation alone is not evidence of a missing IAM permission.
	switch d.Code {
	case "UnauthorizedOperation", "AccessDenied", "AccessDeniedException", "OperationNotPermitted":
	default:
		d.RequiredPermission = ""
	}
	var verified *identityVerifiedEC2Error
	if errors.As(err, &verified) {
		d.IdentityVerified = true
		d.Guidance = "AWS STS verified this credential's identity, but EC2 could not validate service access in the selected region. Check EC2 account activation, billing/verification restrictions and regional access in AWS. This response alone does not prove an incorrect secret key."
	}
	return d
}
