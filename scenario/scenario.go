// Package scenario provides a fluent builder for creating Scrapfly API JS Scenarios.
//
// It abstracts away the complexity of building the raw JSON structure (`[]map[string]interface{}`),
// providing a type-safe and user-friendly interface. The final output is a
// base64 encoded string, ready to be used as the `js_scenario` API parameter.
//
// # Example Usage
//
//	import "your/project/path/scenario"
//
//	func main() {
//		// Create a new scenario builder and chain actions.
//		sc, err := scenario.New().
//			// Fill in login credentials
//			Fill("input[name=username]", "user123").
//			Fill("input[name=password]", "password", scenario.WithFillClear(true)).
//
//			// Click the login button
//			Click("button[type='submit']").
//
//			// Wait for the page to navigate after login
//			WaitForNavigation(scenario.WithNavTimeout(5000)).
//
//			// Wait for a specific element to appear on the new page
//			WaitForSelector("#dashboard", scenario.WithSelectorTimeout(10000)).
//
//			// Execute a script to get the user agent
//			Execute("return navigator.userAgent").
//
//			// Finalize the builder to get the base64 encoded string.
//			Build()
//
//		if err != nil {
//			// Handle error
//			log.Fatalf("Failed to build scenario: %v", err)
//		}
//
//		// Now `sc` can be passed directly to the Scrapfly API client.
//		scrapeConfig := &scrapfly.ScrapeConfig{
//			URL: "https://example.com",
//			RenderJS: true,
//			Country: "us",
//			Unblocker: scrapfly.BoolPtr(true),
//			ProxyPool: scrapfly.PublicResidentialPool,
//			JSScenario: sc,
//		}
//	}
package js_scenario

// JSScenarioStep represents a single step in the JS scenario.
type JSScenarioStep = map[string]any

// ScenarioBuilder manages the construction of a JS scenario.
type ScenarioBuilder struct {
	steps []JSScenarioStep
	err   error
}

// New creates a new, empty instance of the ScenarioBuilder.
func New() *ScenarioBuilder {
	return &ScenarioBuilder{
		steps: make([]JSScenarioStep, 0),
	}
}

// Steps returns the steps of the scenario.
// Use this method when passing the scenario to the ScrapeConfig.JSScenario field.
func (b *ScenarioBuilder) Steps() []JSScenarioStep {
	return b.steps
}

// Build finalizes the scenario, ensure it has no errors and returns the steps.
// If any errors occurred during the building process, they will be returned here.
// If the scenario is empty, it returns nil and no error.
func (b *ScenarioBuilder) Build() ([]JSScenarioStep, error) {
	if b.err != nil {
		return nil, b.err
	}

	if len(b.steps) == 0 {
		return nil, nil // An empty scenario is valid.
	}

	return b.steps, nil
}

// --- Click Action ---

// clickParams holds all parameters for a "click" action.
type clickParams struct {
	Selector           string `json:"selector"`
	IgnoreIfNotVisible bool   `json:"ignore_if_not_visible,omitempty"`
	Multiple           bool   `json:"multiple,omitempty"`
}

// ClickOption is a function that configures a click action.
type ClickOption func(*clickParams)

// WithClickIgnoreIfNotVisible sets the 'ignore_if_not_visible' option for a click.
// If true, the step will be skipped if the element is not visible.
func WithClickIgnoreIfNotVisible(ignore bool) ClickOption {
	return func(p *clickParams) {
		p.IgnoreIfNotVisible = ignore
	}
}

// WithClickMultiple sets the 'multiple' option for a click.
// If true, it will click on all elements matching the selector.
func WithClickMultiple(multiple bool) ClickOption {
	return func(p *clickParams) {
		p.Multiple = multiple
	}
}

// Click adds a step to click on an element matching the given CSS or XPath selector.
func (b *ScenarioBuilder) Click(selector string, opts ...ClickOption) *ScenarioBuilder {
	if b.err != nil {
		return b
	}

	params := &clickParams{Selector: selector}
	for _, opt := range opts {
		opt(params)
	}

	b.steps = append(b.steps, map[string]interface{}{"click": params})
	return b
}

// --- Fill Action ---

// fillParams holds all parameters for a "fill" action.
type fillParams struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Clear    bool   `json:"clear,omitempty"`
}

// FillOption is a function that configures a fill action.
type FillOption func(*fillParams)

// WithFillClear sets the 'clear' option for a fill action.
// If true, the input field will be cleared before typing the value.
func WithFillClear(clear bool) FillOption {
	return func(p *fillParams) {
		p.Clear = clear
	}
}

// Fill adds a step to type a text value into an element matching the selector.
func (b *ScenarioBuilder) Fill(selector, value string, opts ...FillOption) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &fillParams{Selector: selector, Value: value}
	for _, opt := range opts {
		opt(params)
	}
	b.steps = append(b.steps, map[string]interface{}{"fill": params})
	return b
}

// --- Wait Action ---

// Wait adds a step to pause the scenario for a specified duration.
func (b *ScenarioBuilder) Wait(milliseconds int) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	b.steps = append(b.steps, map[string]interface{}{"wait": milliseconds})
	return b
}

// --- Execute Action ---

// executeParams holds all parameters for an "execute" action.
type executeParams struct {
	Script  string `json:"script"`
	Timeout int    `json:"timeout,omitempty"`
}

// ExecuteOption is a function that configures an execute action.
type ExecuteOption func(*executeParams)

// WithExecuteTimeout sets the timeout for a script execution.
func WithExecuteTimeout(milliseconds int) ExecuteOption {
	return func(p *executeParams) {
		p.Timeout = milliseconds
	}
}

// Execute adds a step to run a custom JavaScript snippet in the browser context.
func (b *ScenarioBuilder) Execute(script string, opts ...ExecuteOption) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &executeParams{Script: script}
	for _, opt := range opts {
		opt(params)
	}
	b.steps = append(b.steps, map[string]interface{}{"execute": params})
	return b
}

// --- Wait For Navigation Action ---

// waitForNavParams holds all parameters for a "wait_for_navigation" action.
type waitForNavParams struct {
	Timeout int `json:"timeout,omitempty"`
}

// WaitForNavOption is a function that configures a wait_for_navigation action.
type WaitForNavOption func(*waitForNavParams)

// WithNavTimeout sets the maximum time to wait for a navigation to occur.
func WithNavTimeout(milliseconds int) WaitForNavOption {
	return func(p *waitForNavParams) {
		p.Timeout = milliseconds
	}
}

// WaitForNavigation adds a step that waits for a page navigation to complete.
// This is useful after actions that trigger a page load, like clicking a link or submitting a form.
func (b *ScenarioBuilder) WaitForNavigation(opts ...WaitForNavOption) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &waitForNavParams{}
	for _, opt := range opts {
		opt(params)
	}
	b.steps = append(b.steps, map[string]interface{}{"wait_for_navigation": params})
	return b
}

// --- Wait For Selector Action ---

// SelectorState defines the desired state of an element to wait for.
type SelectorState string

const (
	// SelectorStateVisible waits for the element to be present and visible in the DOM.
	SelectorStateVisible SelectorState = "visible"
	// SelectorStateHidden waits for the element to be removed or hidden from the DOM.
	SelectorStateHidden SelectorState = "hidden"
)

// waitForSelectorParams holds all parameters for a "wait_for_selector" action.
type waitForSelectorParams struct {
	Selector string        `json:"selector"`
	State    SelectorState `json:"state,omitempty"`
	Timeout  int           `json:"timeout,omitempty"`
}

// WaitForSelectorOption is a function that configures a wait_for_selector action.
type WaitForSelectorOption func(*waitForSelectorParams)

// WithSelectorState sets the desired state of the element to wait for (visible or hidden).
func WithSelectorState(state SelectorState) WaitForSelectorOption {
	return func(p *waitForSelectorParams) {
		p.State = state
	}
}

// WithSelectorTimeout sets the maximum time to wait for the selector to reach the desired state.
func WithSelectorTimeout(milliseconds int) WaitForSelectorOption {
	return func(p *waitForSelectorParams) {
		p.Timeout = milliseconds
	}
}

// WaitForSelector adds a step that waits for an element to appear, disappear,
// or reach a specific state.
func (b *ScenarioBuilder) WaitForSelector(selector string, opts ...WaitForSelectorOption) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &waitForSelectorParams{Selector: selector}
	for _, opt := range opts {
		opt(params)
	}
	b.steps = append(b.steps, map[string]interface{}{"wait_for_selector": params})
	return b
}

// --- Scroll Action ---

// scrollParams holds all parameters for a "scroll" action.
type scrollParams struct {
	Element       string `json:"element,omitempty"`
	Selector      string `json:"selector,omitempty"`
	Infinite      int    `json:"infinite,omitempty"`
	ClickSelector string `json:"click_selector,omitempty"`
}

// ScrollOption is a function that configures a scroll action.
type ScrollOption func(*scrollParams)

// WithScrollElement sets a container element (via selector) within which to scroll. Defaults to the page body.
func WithScrollElement(selector string) ScrollOption {
	return func(p *scrollParams) {
		p.Element = selector
	}
}

// WithScrollToSelector sets a target element (via selector) to scroll to. Can also be "bottom".
func WithScrollToSelector(selector string) ScrollOption {
	return func(p *scrollParams) {
		p.Selector = selector
	}
}

// WithScrollInfinite performs a specified number of "infinite scrolls" to the bottom.
func WithScrollInfinite(iterations int) ScrollOption {
	return func(p *scrollParams) {
		p.Infinite = iterations
	}
}

// WithScrollClickAfter performs a click on the given selector after each scroll,
// useful for "load more" buttons in infinite scroll scenarios.
func WithScrollClickAfter(selector string) ScrollOption {
	return func(p *scrollParams) {
		p.ClickSelector = selector
	}
}

// Scroll adds a step to scroll the page or a specific element.
// By default (with no options), it scrolls to the bottom of the page.
func (b *ScenarioBuilder) Scroll(opts ...ScrollOption) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &scrollParams{}
	for _, opt := range opts {
		opt(params)
	}
	b.steps = append(b.steps, map[string]interface{}{"scroll": params})
	return b
}

// --- Condition Action ---

// ConditionAction defines the behavior when a condition is met.
type ConditionAction string

const (
	// ActionContinue continues the scenario if the condition is met.
	ActionContinue ConditionAction = "continue"
	// ActionExitSuccess stops the scenario and marks it as successful if the condition is met.
	ActionExitSuccess ConditionAction = "exit_success"
	// ActionExitFailed stops the scenario and marks it as failed if the condition is met.
	ActionExitFailed ConditionAction = "exit_failed"
)

// conditionParams holds all parameters for a "condition" action.
type conditionParams struct {
	StatusCode    int           `json:"status_code,omitempty"`
	Selector      string        `json:"selector,omitempty"`
	SelectorState SelectorState `json:"selector_state,omitempty"`
	Action        string        `json:"action,omitempty"`
}

// ConditionOnStatusCode adds a condition step that checks the HTTP status code of the response.
func (b *ScenarioBuilder) ConditionOnStatusCode(statusCode int, action ConditionAction) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &conditionParams{
		StatusCode: statusCode,
		Action:     string(action),
	}
	b.steps = append(b.steps, map[string]interface{}{"condition": params})
	return b
}

// ConditionOnSelector adds a condition step based on the presence or absence of an element.
func (b *ScenarioBuilder) ConditionOnSelector(selector string, state SelectorState, action ConditionAction) *ScenarioBuilder {
	if b.err != nil {
		return b
	}
	params := &conditionParams{
		Selector:      selector,
		SelectorState: state,
		Action:        string(action),
	}
	b.steps = append(b.steps, map[string]interface{}{"condition": params})
	return b
}
