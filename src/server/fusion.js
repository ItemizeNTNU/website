/* FusionAuth API client */

import { fetchResource } from '../utils/api';
import fetch from 'node-fetch';
import { inspect } from 'util';

const _fetch = async (path, options) => {
	options = options || {};
	options.headers = options.headers || {};
	options.headers.Authorization = process.env.FUSION_AUTH_API_TOKEN;
	const host = process.env.FUSION_AUTH_HOST;
	const res = await fetchResource(path, { fetch, host, ...options });
	if (res.error) {
		res.error = { status: res.status, json: res.json, message: `An error occured while communicating with ${host}` };
		if (Array.isArray(res.error.json.generalErrors)) {
			res.error.message = res.error.json.generalErrors[0].message;
		}
		if (res.error.json.fieldErrors) {
			const keys = Object.keys(res.error.json.fieldErrors);
			if (keys) {
				res.error.message = res.error.json.fieldErrors[keys[0]][0].message;
			}
		}
		console.error('FusionAuth [Error]:', inspect(res.error, false, null, true));
	}
	res.client = res.error ? { message: res.error.message } : { message: 'Success' };
	return res;
};

export const createUser = async (user) => {
	return await _fetch('/api/user', { method: 'POST', json: user });
};

export const updateUser = async (id, user) => {
	return await _fetch(`/api/user/${id}`, { method: 'PATCH', json: { user: user } });
};

export const getUser = async (id) => {
	const res = await _fetch(`/api/user/${id}`);
	if (res.json?.user) {
		res.json = res.json.user;
	}
	return res;
};

export const searchUsers = async (queryString) => {
	const res = await _fetch(`/api/user/search?${queryString}`);
	if (res.json?.users) {
		res.json = res.json.users;
	}
	return res;
};

export const getAllApplications = async () => {
	const res = await _fetch(`/api/application`);
	if (res.json?.applications) {
		res.json = res.json.applications;
	}
	return res;
};

export const getAllGroups = async () => {
	const res = await _fetch(`/api/group`);
	if (res.json?.groups) {
		res.json = res.json.groups;
	}
	return res;
};

export default { createUser, updateUser, getUser, searchUsers, getAllApplications, getAllGroups };
