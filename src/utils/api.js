const defFetchOptions = {
	host: '',
	method: 'GET',
	json: '',
	errorText: 'Unable to fetch resource: ERROR',
	fetch: undefined,
	headers: {}
}
export const fetchResource = async (path, options = defFetchOptions) => {
	options = { ...defFetchOptions, ...options };
	path = options.host + path
	const settings = {
		method: options.method,
		headers: options.headers,
	};
	if (options.json) {
		settings.body = JSON.stringify(options.json);
		settings.headers['Content-Type'] = 'application/json';
	}
	let resp = {};
	try {
		if (options.fetch) {
			resp = await options.fetch(path, settings);
		} else {
			resp = await fetch(path, settings);
		}
		try {
			resp.text = await resp.text();
		} catch (err) { }
		try {
			resp.json = JSON.parse(resp.text);
		} catch (err) { }
		if (!resp.ok) {
			resp.error = resp.json?.message || (typeof resp.text == 'string' ? resp.text : resp.statusText);
		}
	} catch (error) {
		resp.error = error;
		resp.exception = true;
		console.log('An exception occured while fetching ' + path, error);
	}
	if (typeof resp.error == 'string' && options.errorText) {
		resp.error = options.errorText.replace(/\bERROR\b/g, resp.error);
	}
	return resp;
};

export const registerUser = async (user, options = {}) => {
	return await fetchResource('/api/user', { method: 'PUT', json: user, errorText: 'ERROR', ...options });
}

export default { registerUser };