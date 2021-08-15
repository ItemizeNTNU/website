/* FusionAuth API client */

import { fetchResource } from "../utils/api"
import fetch from "node-fetch"
import { inspect } from 'util'

const _fetch = async (path, options) => {
	options.headers = options.headers || {}
	options.headers.Authorization = process.env.AUTH_API_TOKEN
	const host = 'https://auth.itemize.no'
	const res = await fetchResource(path, { fetch, host, ...options })
	if (res.error) {
		res.error = { status: res.status, json: res.json, message: `An error occured while communicating with ${host}` };
		if (Array.isArray(res.error.json.generalErrors)) {
			res.error.message = res.error.json.generalErrors[0].message
		}
		if (res.error.json.fieldErrors) {
			const keys = Object.keys(res.error.json.fieldErrors);
			if (keys) {
				res.error.message = res.error.json.fieldErrors[keys[0]][0].message;
			}
		}
		console.error('FusionAuth [Error]:', inspect(res.error, false, null, true));
	}
	res.client = res.error ? { message: res.error.message } : { message: 'Success' }
	return res;
}

export const createUser = async user => {
	return await _fetch('/api/user', { method: 'POST', json: user })
}

export default { createUser }