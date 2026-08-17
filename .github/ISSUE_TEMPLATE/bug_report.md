name: Bug Report
description: Report something that is not working as expected
labels: [bug]
body:
  - type: markdown
    attributes:
      value: |
        Thanks for taking the time to report a bug! Please fill out the details below.
  - type: textarea
    id: what-happened
    attributes:
      label: What happened?
      description: A clear description of the bug.
      placeholder: Describe the unexpected behavior...
    validations:
      required: true
  - type: textarea
    id: expected
    attributes:
      label: What did you expect to happen?
    validations:
      required: true
  - type: textarea
    id: steps
    attributes:
      label: Steps to reproduce
      placeholder: |
        1. Run `...`
        2. See error
    validations:
      required: true
  - type: input
    id: version
    attributes:
      label: Version / commit
      description: Binary version or git commit you are running
      placeholder: e.g. v0.1.0 or 6b4fb4e
  - type: textarea
    id: logs
    attributes:
      label: Relevant logs
      render: shell
