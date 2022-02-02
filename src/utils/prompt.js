import { writable } from 'svelte/store';

let prompts_value;
export let prompts = writable([]);
prompts.subscribe(ps => {
	prompts_value = ps.filter(p => p.show)
	if (ps.length != prompts_value.length) {
		prompts.set(prompts_value);
	}
});


export function prompt_raw(prompt) {
	return new Promise((accept, close) => {
		prompt.accept = () => accept(prompt.inputs.length == 1 ? prompt.inputs[0].value : prompt.inputs);
		prompt.close = close;
		prompts.set([...prompts_value, prompt]);
	})
}

/**
 * Async pop-up modal asking user for a set of inputs.
 * @param {String} title Title to ask the user
 * @param {String | String[] | Object[]} inputs Mixed list of either strings or {name, value} objects. Pure strings get mapped to {name, ''}
 * @param {String} [body] Optional body text above inputs. **Warning Parsed as HTML! Never pass in unsanitized user input!**.
 * @param {String} [acceptText] Optional text for the accept button. Default 'Submit'. Set to falsely to disable accept button.
 * @param {Function<Strign>} [validate] Optional function which gets passed in the current inputs on each change which when returns true enables the acceptButton.
 * @returns {Object[]} Returns a list of inputs on the form [{name, value}]. If only 1 input was asked, then it returns inputs[0].value
 */
export function prompt(title, inputs, body, acceptText, validate, pre) {
	if (typeof acceptText == 'undefined') {
		acceptText = 'Submit';
	}
	if (!validate) validate = () => true;
	if (typeof inputs == 'string') inputs = [{ name: inputs, value: '' }];
	inputs = inputs.map(input => {
		if (typeof (input) == 'string') {
			return { name: input, value: '' }
		} else if (Array.isArray(input)) {
			return { name: input[0], value: input[1] }
		} else {
			return input;
		}
	});
	const prompt = {
		inputs,
		show: true,
		title,
		body,
		acceptText,
		acceptDisable: !validate(inputs),
		validate,
		pre,
	}
	return prompt_raw(prompt);
}

export default prompt;

const true_false_wrap = (promise) => new Promise((res) => promise.then(() => res(true)).catch(() => res(false)))

/**
 * Pop up confirm modal
 * @param {String} title Title question to ask user
 * @returns async true/false
 */
export function confirm(title) {
	return true_false_wrap(prompt(title, [], '', 'Confirm'))
}

/**
 * Pop up confirm modal where user needs to retype the `text` to confirm the action
 * @param {String} title Title question to ask user
 * @param {String} text Text the user needs to type to confirm
 * @returns async true/false
 */
export function confirm_strong(title, text) {
	return true_false_wrap(prompt(title, [''], `Type <code class="error">${text}</code> to confirm:`, 'Confirm', (inputs => inputs[0].value == text)));
}


export function alert(title, body, pre) {
	return true_false_wrap(prompt(title, [], body, '', false, pre))
}

