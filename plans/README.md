# Plans

This is where we add files that define different test scenarios we want to run. For this walkthrough, we've only included one plan file so you have a sense of how it works. The Github Action workflow file currently sets the plan path to the one plan file in this directory, but if you had more plan files, you could pass in the path of whatever plan you wanted to run.

- [agent-endpoints-with-traffic-policy.yaml](./agent-endpoints-with-traffic-policy.yaml): This plan spins up the 2048 game using agent endpoint CRDs and applies a traffic policy with an `oauth` action. The expected HTTP status is 302.
