---
title: Chainsaw for MPC
author: Francesco Ilario
---

What's Kyverno Chainsaw
===

<!-- font_size: 3 -->

Kyverno Chainsaw is a tool primarily developed to run end-to-end tests in Kubernetes clusters.

It is meant to test Kubernetes operators work as expected by running a sequence of steps and asserting various conditions.

* **Easy to use**
* **Comprehensive reporting**
* **Resource Templating**
* **Positive testing**
* **Negative testing**
* **Multi cluster**

<!-- 
speaker_note: |
    * **Easy to use**: A YAML file to define the steps of a test.
    * **Comprehensive reporting**: Generates JUnit compatible reports.
    * **Resource Templating**: built-in support to easily describe complex test scenarios.
    * **Positive testing**: Create, update, delete resources and assert your controller reconciles the desired and observed states in the expected way.
    * **Negative testing**: Try submitting invalid resources, invalid changes, or other disallowed actions and make sure they are rejected.
    * **Multi cluster**: Native support for tests involving multiple clusters, either static or dynamically created ones.
-->


<!-- end_slide -->

Testing with Chainsaw
===
<!-- font_size: 3 -->

To create a Chainsaw test all you need to do is to create one (or more) YAML file(s).

The recommended approach is to create one folder per test, with a `chainsaw-test.yaml` file containing one (or more) test definition(s).


<!-- end_slide -->

How it can help us on MPC
===
<!-- font_size: 3 -->

Kyverno Chainsaw can be a quick and easy solution for us to implement some blackbox tests.

Tests should be abstracted from MPC's internals and guarantee that we don't break users' experience.


<!-- end_slide -->

Testing MPC's workflows
===
<!-- font_size: 3 -->

Let's start with the simplest test we can think of for MPC:

* When a arch-specific TaskRun is created
* Then the TaskRun is correctly put in execution
* Then the TaskRun succeed
    * the TaskRun is able to access the secret

<!-- end_slide -->

Specification
===

```yaml
apiVersion: chainsaw.kyverno.io/v1alpha1
kind: Test
metadata:
  name: local-succeeded-taksrun
spec:
  description: |
    a valid taskrun is reconciled and successfully completed
  steps:
  - name: when-taskrun-is-created
    try:
    - apply:
        file: ../resources/actual_succeeding_tekton_taskrun.yaml
  - name: then-taskrun-is-running
    try:
    - assert:
        file: ../resources/expected_tekton_taskrun_running.yaml
        bindings:
        - name: taskRunName
          value: local-succeeded
  - name: then-taskrun-succeeded
    try:
    - assert:
        file: ../resources/expected_tekton_taskrun_succeeded.yaml
```

> specification at `acceptance/.chainsaw-test/local/succeeded-taskrun/chainsaw-test.yaml`

<!-- end_slide -->

Demo
===
<!-- font_size: 3 -->

```bash +exec +acquire_terminal 
/// clear
/// echo '$ chainsaw test .chainsaw-test/local/succeeded-taskrun' && read
/// echo
chainsaw test .chainsaw-test/local/succeeded-taskrun
/// echo
/// echo -n "Press Enter to go back to the presentation... "
/// read
```

> Press Ctrl+e to run the demo

<!-- end_slide -->

Steps
===
<!-- font_size: 3 -->

The scenario we discussed was simplified for demo purposes. It only relies on the two steps `apply` and `assert`.

Chainsaw provides several [operations](https://kyverno.github.io/chainsaw/latest/operations/), among which the generic **Command** one.


<!-- end_slide -->

Adopters
===
<!-- font_size: 3 -->

* [infra-deployments](https://github.com/redhat-appstudio/infra-deployments/tree/main/components/policies)
* [Kyverno](https://github.com/kyverno/kyverno/tree/main/test)
* [OpenTelemetry operator](https://github.com/open-telemetry/opentelemetry-operator/tree/main/tests)
* [Grafana Operator](https://github.com/grafana/grafana-operator/tree/master/tests/e2e)
* [Opstree's RedisOperator](https://github.com/OT-CONTAINER-KIT/redis-operator/tree/main/tests)


<!-- end_slide -->

Conclusions
===
<!-- font_size: 3 -->

<!-- column_layout: [5, 5] -->

<!-- column: 0 -->

# Pros

* **Simplicity**
    * Quick adoption
    * Easy to read / write

<!-- new_lines: 1 -->

* **Hard to Write Complex tests**
    * Easier to avoid trying to implement overly complex tests

<!-- new_lines: 1 -->

<!-- column: 1 -->


# Cons

* **Non-GA Release**
    * Currently at version 0.2.3

<!-- new_lines: 2 -->

* **Hard to Write Complex tests**
    * implement complex scenarios is not trivial

<!-- reset_layout -->
