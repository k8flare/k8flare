package validating

func CompilePolicy(policy *Policy) Validator {
	return compilePolicy(policy)
}
