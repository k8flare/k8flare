package mutating

func CompilePolicy(policy *Policy) PolicyEvaluator {
	return compilePolicy(policy)
}
