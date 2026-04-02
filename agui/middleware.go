package agui

// Middleware wraps an Agent, returning a new Agent.
type Middleware func(next Agent) Agent

// Chain composes middlewares in order: Chain(a, b, c)(agent) = a(b(c(agent))).
func Chain(middlewares ...Middleware) Middleware {
	return func(agent Agent) Agent {
		for i := len(middlewares) - 1; i >= 0; i-- {
			agent = middlewares[i](agent)
		}
		return agent
	}
}
