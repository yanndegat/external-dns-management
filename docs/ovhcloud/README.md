# OVHCloud DNS Provider

This DNS provider allows you to create and manage DNS entries with [OVHCloud](https://www.ovh.com/). 

## Generate API Credentials

Requests to OVHcloud APIs require a set of secrets keys and the definition of the API end point. 
See [First Steps with the API](https://docs.ovh.com/gb/en/customer/first-steps-with-ovh-api/) for a detailed explanation.

Besides the API end-point, the required keys are the `application_key`, the `application_secret`, and the `consumer_key`.
These keys can be generated via the [OVH token generation page](https://api.ovh.com/createToken/?GET=/*&POST=/*&PUT=/*&DELETE=/*). 

## Required permissions

There are no special permissions for the access tokens.

## Using the Access Token

Create a `Secret` resource with the data fields `OVH_ENDPOINT`, `OVH_APPLICATION_KEY`,_`APPLICATION_SECRET` and `OVH_CONSUMER_KEY`.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ovhcloud-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with values
  # see https://docs.ovh.com/gb/en/customer/first-steps-with-ovh-api/
  OVH_ENDPOINT: ...
  OVH_APPLICATION_KEY: ...
  OVH_APPLICATION_SECRET: ...
  OVH_CONSUMER_KEY: ...
``` 
