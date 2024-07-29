package env

import (
	"os"
	"strings"
)

const (
	OTEL_EXPORTER_OTLP_GRPC_ENDPOINT = "OTEL_EXPORTER_OTLP_GRPC_ENDPOINT"
	OTEL_EXPORTER_OTLP_HTTP_ENDPOINT = "OTEL_EXPORTER_OTLP_HTTP_ENDPOINT"
	OTEL_EXPORTER_OPAMP_ENDPOINT     = "OTEL_EXPORTER_OPAMP_ENDPOINT"
	OTEL_AUTO_SERVICE_NAME           = "OTEL_AUTO_SERVICE_NAME"
)

type CustomOtlpENDPOINT struct {
	OtlpHTTPEndpoint string
	OtlpGrpcEndpoint string
	OpAMPEndpoint    string

	AutoServiceName bool
}

func LoadCustomEndpoint() CustomOtlpENDPOINT {
	httpEndpoint, ok := os.LookupEnv(OTEL_EXPORTER_OTLP_HTTP_ENDPOINT)
	if !ok {
		httpEndpoint = ""
	}

	grpcEndpoint, ok := os.LookupEnv(OTEL_EXPORTER_OTLP_GRPC_ENDPOINT)
	if !ok {
		grpcEndpoint = ""
	}

	opampEndpoint, ok := os.LookupEnv(OTEL_EXPORTER_OPAMP_ENDPOINT)
	if !ok {
		opampEndpoint = ""
	}

	autoServiceOption := true
	autoServiceName, ok := os.LookupEnv(OTEL_AUTO_SERVICE_NAME)
	if !ok || strings.ToLower(autoServiceName) != "true" {
		autoServiceOption = false
	}

	return CustomOtlpENDPOINT{
		OtlpHTTPEndpoint: httpEndpoint,
		OtlpGrpcEndpoint: grpcEndpoint,
		OpAMPEndpoint:    opampEndpoint,
		AutoServiceName:  autoServiceOption,
	}
}
