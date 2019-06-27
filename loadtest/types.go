package loadtest

type LoadTestApp struct {
	Ingress LoadTestAppIngress `json:"ingress"`
}

type LoadTestAppIngress struct {
	Hosts []string `json:"hosts"`
}

type LoadTestValues struct {
	Auth LoadTestValuesAuth `json:"auth"`
	Test LoadTestValuesTest `json:"test"`
}

type LoadTestValuesAuth struct {
	Token string `json:"token"`
}

type LoadTestValuesTest struct {
	Endpoint string `json:"endpoint"`
}
