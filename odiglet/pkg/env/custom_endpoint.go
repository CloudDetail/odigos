package env

import (
	"os"
)

const (
	OTEL_EXPORTER_OTLP_GRPC_ENDPOINT = "OTEL_EXPORTER_OTLP_GRPC_ENDPOINT"
	OTEL_EXPORTER_OTLP_HTTP_ENDPOINT = "OTEL_EXPORTER_OTLP_HTTP_ENDPOINT"
	OTEL_EXPORTER_OPAMP_ENDPOINT     = "OTEL_EXPORTER_OPAMP_ENDPOINT"
)

type CustomOtlpENDPOINT struct {
	OtlpHTTPEndpoint string
	OtlpGrpcEndpoint string
	OpAMPEndpoint    string
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

	return CustomOtlpENDPOINT{
		OtlpHTTPEndpoint: httpEndpoint,
		OtlpGrpcEndpoint: grpcEndpoint,
		OpAMPEndpoint:    opampEndpoint,
	}
}
