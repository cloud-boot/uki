// Minimal demo plan showing the subplan target type.
//
// This file is the typical "thin air-gapped boot.iso" pattern:
// the iso embeds *only* this plan (via `cloud-boot build
// --plan-file examples/plan-subplan-demo.hcl`), which references
// the actual catalog via an OCI plan that lives on a registry
// and evolves independently of the bootable artifact.
//
// At boot:
//   1. init reads /etc/cloud-boot/plan.hcl (= this file).
//   2. The "live" target is picked (cmdline `cloudboot.target=live`
//      or default_target).
//   3. init sees t.Subplan != "" — fetches the OCI ref, decodes the
//      inner plan, and runs target selection on it (with
//      cloudboot.target cleared so the inner plan re-prompts or
//      uses its own default).
//   4. The inner plan resolves to a normal OCI/disk target;
//      reboot sink stages it; firmware boots it.
//
// Stack subplans by giving the inner plan ITS OWN subplan targets
// — up to 8 levels deep (init's maxSubplanDepth). Useful for
// federation hierarchies ("region → datacentre → rack → host").

default_target = "live"

target "live" {
  label   = "Live fleet (subplan via OCI)"
  subplan = "192.168.64.1:5000/boot/inner-plan:v1"
}
