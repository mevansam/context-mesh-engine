package plan.outbound

import rego.v1

default allow := false

allow if is_object(input.outputs)

# Browse responses should not include photo URLs when present.
redact := ["/pet/photoUrls"] if {
	object.get(object.get(input.inputs, "policyHints", {}), "mode", "") == "read"
	input.workflowId == "retrievePet"
}
