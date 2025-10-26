package js_scenario

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

const jsScenarioSchemaString = `
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://scrapfly.io/schemas/js_scenario.json",
  "title": "Scrapfly JS Scenario",
  "description": "A schema for validating a sequence of browser actions (JS Scenario) for the Scrapfly API.",
  "type": "array",
  "items": {
    "$ref": "#/$defs/scenarioStep"
  },
  "$defs": {
    "conditionAction": {
      "type": "string",
      "enum": [
        "continue",
        "exit_success",
        "exit_failed"
      ],
      "default": "continue"
    },
    "scenarioStep": {
      "title": "Scenario Step",
      "description": "A single step in a JS Scenario. It must be an object with exactly one key, which defines the action to perform.",
      "type": "object",
      "oneOf": [
        {
          "$ref": "#/$defs/clickStep"
        },
        {
          "$ref": "#/$defs/fillStep"
        },
        {
          "$ref": "#/$defs/conditionStep"
        },
        {
          "$ref": "#/$defs/waitStep"
        },
        {
          "$ref": "#/$defs/scrollStep"
        },
        {
          "$ref": "#/$defs/executeStep"
        },
        {
          "$ref": "#/$defs/waitForNavigationStep"
        },
        {
          "$ref": "#/$defs/waitForSelectorStep"
        }
      ]
    },
    "clickStep": {
      "title": "Click Step",
      "type": "object",
      "properties": {
        "click": {
          "type": "object",
          "properties": {
            "selector": {
              "type": "string",
              "minLength": 1
            },
            "ignore_if_not_visible": {
              "type": "boolean",
              "default": false
            },
            "multiple": {
              "type": "boolean",
              "default": false
            }
          },
          "required": [
            "selector"
          ],
          "additionalProperties": false
        }
      },
      "required": [
        "click"
      ],
      "additionalProperties": false
    },
    "fillStep": {
      "title": "Fill Step",
      "type": "object",
      "properties": {
        "fill": {
          "type": "object",
          "properties": {
            "selector": {
              "type": "string",
              "minLength": 1
            },
            "value": {
              "type": "string"
            },
            "clear": {
              "type": "boolean",
              "default": false
            }
          },
          "required": [
            "selector",
            "value"
          ],
          "additionalProperties": false
        }
      },
      "required": [
        "fill"
      ],
      "additionalProperties": false
    },
    "conditionStep": {
      "title": "Condition Step",
      "type": "object",
      "properties": {
        "condition": {
          "description": "Defines a condition that must be met to continue the scenario. The condition is based on either the response status code or the state of a selector.",
          "oneOf": [
            {
              "title": "Status Code Condition",
              "type": "object",
              "properties": {
                "status_code": {
                  "type": "integer"
                },
                "action": {
                  "$ref": "#/$defs/conditionAction"
                }
              },
              "required": [
                "status_code"
              ],
              "additionalProperties": false
            },
            {
              "title": "Selector Condition",
              "type": "object",
              "properties": {
                "selector": {
                  "type": "string",
                  "minLength": 1
                },
                "selector_state": {
                  "type": "string",
                  "enum": [
                    "existing",
                    "not_existing"
                  ],
                  "default": "existing"
                },
                "action": {
                  "$ref": "#/$defs/conditionAction"
                }
              },
              "required": [
                "selector"
              ],
              "additionalProperties": false
            }
          ]
        }
      },
      "required": [
        "condition"
      ],
      "additionalProperties": false
    },
    "waitStep": {
      "title": "Wait Step",
      "type": "object",
      "properties": {
        "wait": {
          "type": "integer",
          "minimum": 0,
          "description": "Duration to wait in milliseconds."
        }
      },
      "required": [
        "wait"
      ],
      "additionalProperties": false
    },
    "scrollStep": {
      "title": "Scroll Step",
      "type": "object",
      "properties": {
        "scroll": {
          "type": "object",
          "properties": {
            "element": {
              "type": "string",
              "default": "body",
              "minLength": 1
            },
            "selector": {
              "type": "string",
              "default": "bottom",
              "minLength": 1
            },
            "infinite": {
              "type": "integer",
              "minimum": 0,
              "default": 0
            },
            "click_selector": {
              "type": "string",
              "minLength": 1
            }
          },
          "additionalProperties": false
        }
      },
      "required": [
        "scroll"
      ],
      "additionalProperties": false
    },
    "executeStep": {
      "title": "Execute Step",
      "type": "object",
      "properties": {
        "execute": {
          "type": "object",
          "properties": {
            "script": {
              "type": "string",
              "minLength": 1
            },
            "timeout": {
              "type": "integer",
              "minimum": 0,
              "default": 3000
            }
          },
          "required": [
            "script"
          ],
          "additionalProperties": false
        }
      },
      "required": [
        "execute"
      ],
      "additionalProperties": false
    },
    "waitForNavigationStep": {
      "title": "WaitForNavigation Step",
      "type": "object",
      "properties": {
        "wait_for_navigation": {
          "type": "object",
          "properties": {
            "timeout": {
              "type": "integer",
              "minimum": 0,
              "default": 1000
            }
          },
          "additionalProperties": false
        }
      },
      "required": [
        "wait_for_navigation"
      ],
      "additionalProperties": false
    },
    "waitForSelectorStep": {
      "title": "WaitForSelector Step",
      "type": "object",
      "properties": {
        "wait_for_selector": {
          "type": "object",
          "properties": {
            "selector": {
              "type": "string",
              "minLength": 1
            },
            "state": {
              "type": "string",
              "enum": [
                "visible",
                "hidden"
              ],
              "default": "visible"
            },
            "timeout": {
              "type": "integer",
              "minimum": 0,
              "default": 5000
            }
          },
          "required": [
            "selector"
          ],
          "additionalProperties": false
        }
      },
      "required": [
        "wait_for_selector"
      ],
      "additionalProperties": false
    }
  }
}
`

const jsScenarioSchemaFlattenedString = `
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://scrapfly.io/schemas/js_scenario.json",
  "title": "Scrapfly JS Scenario",
  "description": "A schema for validating a sequence of browser actions (JS Scenario) for the Scrapfly API.",
  "type": "array",
  "items": {
    "title": "Scenario Step",
    "description": "A single step in a JS Scenario. It must be an object with exactly one key, which defines the action to perform.",
    "type": "object",
    "oneOf": [
      {
        "title": "Click Step",
        "type": "object",
        "properties": {
          "click": {
            "type": "object",
            "properties": {
              "selector": {
                "type": "string",
                "minLength": 1
              },
              "ignore_if_not_visible": {
                "type": "boolean",
                "default": false
              },
              "multiple": {
                "type": "boolean",
                "default": false
              }
            },
            "required": [
              "selector"
            ],
            "additionalProperties": false
          }
        },
        "required": [
          "click"
        ],
        "additionalProperties": false
      },
      {
        "title": "Fill Step",
        "type": "object",
        "properties": {
          "fill": {
            "type": "object",
            "properties": {
              "selector": {
                "type": "string",
                "minLength": 1
              },
              "value": {
                "type": "string"
              },
              "clear": {
                "type": "boolean",
                "default": false
              }
            },
            "required": [
              "selector",
              "value"
            ],
            "additionalProperties": false
          }
        },
        "required": [
          "fill"
        ],
        "additionalProperties": false
      },
      {
        "title": "Condition Step",
        "type": "object",
        "properties": {
          "condition": {
            "description": "Defines a condition that must be met to continue the scenario. The condition is based on either the response status code or the state of a selector.",
            "oneOf": [
              {
                "title": "Status Code Condition",
                "type": "object",
                "properties": {
                  "status_code": {
                    "type": "integer"
                  },
                  "action": {
                    "type": "string",
                    "enum": [
                      "continue",
                      "exit_success",
                      "exit_failed"
                    ],
                    "default": "continue"
                  }
                },
                "required": [
                  "status_code"
                ],
                "additionalProperties": false
              },
              {
                "title": "Selector Condition",
                "type": "object",
                "properties": {
                  "selector": {
                    "type": "string",
                    "minLength": 1
                  },
                  "selector_state": {
                    "type": "string",
                    "enum": [
                      "existing",
                      "not_existing"
                    ],
                    "default": "existing"
                  },
                  "action": {
                    "type": "string",
                    "enum": [
                      "continue",
                      "exit_success",
                      "exit_failed"
                    ],
                    "default": "continue"
                  }
                },
                "required": [
                  "selector"
                ],
                "additionalProperties": false
              }
            ]
          }
        },
        "required": [
          "condition"
        ],
        "additionalProperties": false
      },
      {
        "title": "Wait Step",
        "type": "object",
        "properties": {
          "wait": {
            "type": "integer",
            "minimum": 0,
            "description": "Duration to wait in milliseconds."
          }
        },
        "required": [
          "wait"
        ],
        "additionalProperties": false
      },
      {
        "title": "Scroll Step",
        "type": "object",
        "properties": {
          "scroll": {
            "type": "object",
            "properties": {
              "element": {
                "type": "string",
                "default": "body",
                "minLength": 1
              },
              "selector": {
                "type": "string",
                "default": "bottom",
                "minLength": 1
              },
              "infinite": {
                "type": "integer",
                "minimum": 0,
                "default": 0
              },
              "click_selector": {
                "type": "string",
                "minLength": 1
              }
            },
            "additionalProperties": false
          }
        },
        "required": [
          "scroll"
        ],
        "additionalProperties": false
      },
      {
        "title": "Execute Step",
        "type": "object",
        "properties": {
          "execute": {
            "type": "object",
            "properties": {
              "script": {
                "type": "string",
                "minLength": 1
              },
              "timeout": {
                "type": "integer",
                "minimum": 0,
                "default": 3000
              }
            },
            "required": [
              "script"
            ],
            "additionalProperties": false
          }
        },
        "required": [
          "execute"
        ],
        "additionalProperties": false
      },
      {
        "title": "WaitForNavigation Step",
        "type": "object",
        "properties": {
          "wait_for_navigation": {
            "type": "object",
            "properties": {
              "timeout": {
                "type": "integer",
                "minimum": 0,
                "default": 1000
              }
            },
            "additionalProperties": false
          }
        },
        "required": [
          "wait_for_navigation"
        ],
        "additionalProperties": false
      },
      {
        "title": "WaitForSelector Step",
        "type": "object",
        "properties": {
          "wait_for_selector": {
            "type": "object",
            "properties": {
              "selector": {
                "type": "string",
                "minLength": 1
              },
              "state": {
                "type": "string",
                "enum": [
                  "visible",
                  "hidden"
                ],
                "default": "visible"
              },
              "timeout": {
                "type": "integer",
                "minimum": 0,
                "default": 5000
              }
            },
            "required": [
              "selector"
            ],
            "additionalProperties": false
          }
        },
        "required": [
          "wait_for_selector"
        ],
        "additionalProperties": false
      }
    ]
  }
}
`

// JsScenarioSchemaFlattened is the flattened schema for the JS Scenario.
// Use it with more "humble" models or where compatibility with recent meta-schemas if not available.
var JsScenarioSchemaFlattened *jsonschema.Schema

// JsScenarioSchema is the schema for the JS Scenario.
// Use it with more capable models or where compatibility with recent meta-schemas is required.
var JsScenarioSchema *jsonschema.Schema

func init() {
	err := json.Unmarshal([]byte(jsScenarioSchemaString), &JsScenarioSchema)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal([]byte(jsScenarioSchemaFlattenedString), &JsScenarioSchemaFlattened)
	if err != nil {
		panic(err)
	}
}
