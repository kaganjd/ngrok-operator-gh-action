# Running the ngrok Kubernetes Operator in a Github Action

This repo uses Github Actions to deploy the ngrok Kubernetes Operator in a single-node Kubernetes cluster.

The `kubernetes.yml` in `.github/workflows` defines the setup. When run, it:

1. Starts a Kubernetes cluster
1. Installs Helm, the Kubernetes package manager
1. Installs the ngrok Helm chart
1. Deploys the ngrok Helm chart
1. Installs the app defined in `charts/app` and runs the specified test plan from the `plans/` directory
1. Uninstalls the app Helm chart and cleans up ngrok resources

## Run it yourself

1. Fork and clone this repository
1. Add ngrok credentials to your repository's secrets. You'll need `NGROK_AUTHTOKEN` and `NGROK_API_KEY`, which you can create in your ngrok dashboard. You'll then add those here: https://github.com/{YOUR-REPO}/ngrok-operator-gh-action/settings/secrets/actions
1. Reserve an ngrok domain in your dashboard if you haven't already, and set it as an environment variable called `NGROK_URL`. You can set this in your Github repo settings here: https://github.com/{YOUR-REPO}/ngrok-operator-gh-action/settings/variables/actions
1. Run the Github Action here by clicking "Run workflow" on the right side: https://github.com/{YOUR-REPO}/ngrok-operator-gh-action/actions/workflows/kubernetes.yml
1. Click into the Action run to watch each step of the process happen. In another browser tab, open your ngrok dashboard to https://dashboard.ngrok.com/endpoints to see your agent endpoint get created with your domain. You can also visit https://dashboard.ngrok.com/observability/traffic-inspector to watch the request from the test plan come in.
