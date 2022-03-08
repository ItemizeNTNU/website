const defFetchOptions = {
	host: '',
	method: 'GET',
	json: '',
	/** Only works for one layer! */
	urlData: '',
	errorText: 'Unable to fetch resource: ERROR',
	fetch: undefined,
	headers: {},
	debug: false
};
export const fetchResource = async (path, options = defFetchOptions) => {
	options = { ...defFetchOptions, ...options };
	path = options.host + path;
	const settings = {
		method: options.method,
		headers: options.headers
	};
	if (options.urlData) {
		settings.body = Object.keys(options.urlData)
			.map((k) => encodeURIComponent(k) + '=' + encodeURIComponent(options.urlData[k]))
			.join('&');
		settings.headers['Content-Type'] = 'application/x-www-form-urlencoded';
	}
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
		} catch (err) {
			// try to read body if any
		}
		try {
			resp.json = JSON.parse(resp.text);
		} catch (err) {
			// try to convert to json, if json format
		}
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
	if (options.debug) {
		console.debug(options.method, path, settings);
		console.debug(resp);
	}
	return resp;
};

export const registerUser = async (user, options = {}) => {
	return await fetchResource('/api/user', { method: 'PUT', json: user, errorText: 'ERROR', ...options });
};

export const getUser = async (id, options) => {
	return await fetchResource(`/api/user/${id}`, options);
};

export const getEvents = async (old, options) => {
	return await fetchResource(`/api/events?old=${old}`, options);
};

export const postEvent = async (event) => {
	return await fetchResource('/api/events', { method: 'POST', json: event, errorText: 'ERROR' });
};

export const deleteEvent = async (eventId) => {
	return await fetchResource(`/api/events/${eventId}`, { method: 'DELETE', errorText: 'ERROR' });
};

export default { registerUser, getUser, getEvents, postEvent, deleteEvent };
