name: Feature Request
description: Suggest an idea or enhancement
labels: [enhancement]
body:
  - type: textarea
    id: problem
    attributes:
      label: What problem does this solve?
      description: Is your feature request related to a problem? Please describe.
    validations:
      required: true
  - type: textarea
    id: solution
    attributes:
      label: Proposed solution
      description: A clear description of what you want to happen.
    validations:
      required: true
  - type: textarea
    id: alternatives
    attributes:
      label: Alternatives considered
