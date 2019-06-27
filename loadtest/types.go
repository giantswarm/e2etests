package loadtest

type LoadTestApp struct {
	Ingress LoadTestAppIngress `json:"ingress"`
}

type LoadTestAppIngress struct {
	Hosts []string `json:"hosts"`
}

type LoadTestValues struct {
	Test LoadTestValuesTest `json:"test"`
}

type LoadTestValuesTest struct {
	Endpoint string `json:"endpoint"`
}
