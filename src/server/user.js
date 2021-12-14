import express from 'express';
import joi from 'joi';
import { permission, removeEmptyObjects } from './utils';
import fusion from '../server/fusion';
import discord from './discord';

export const router = express.Router();

const DEFAULT_PROFILE_IMAGE = 'https://itemize.no/logo-512.png'; // TODO: change to proper default profile

const validateNTNUEmail = (e) => {
	if (String(e).toLowerCase().endsWith('@stud.ntnu.no')) {
		throw Error('Vennligst ikke bruk din stud e-post adresse, da du mister tilgang til denne etter fullført utdannelse.');
	}
	return e;
};

const UserSchema = joi.object({
	fullName: joi.string().min(3).max(64).required(),
	email: joi.string().email().custom(validateNTNUEmail).required(),
	data: {
		displayName: joi.string().min(3).max(32).required(),
		type: joi.string().allow('student', 'alumni', 'employee').required(),
		study: {
			program: joi.when('$type', { is: ['alumni', 'student'], then: joi.string().min(3).max(64).required(), otherwise: joi.strip() }),
			year: joi.when('$type', { is: 'student', then: joi.number().integer().min(1).max(100).required(), otherwise: joi.strip() })
		},
		alumni: {
			joinYear: joi.when('$type', { is: 'alumni', then: joi.number().integer().min(2014).max(new Date().getFullYear()).required(), otherwise: joi.strip() })
		},
		employee: {
			title: joi.when('$type', { is: 'employee', then: joi.string().min(3).max(64).required(), otherwise: joi.strip() })
		}
	}
});

const validate_user = (data) => {
	const user = UserSchema.validate(data, { abortEarly: true, convert: true, stripUnknown: true, context: { type: data.data?.type } });
	if (!user.error) {
		removeEmptyObjects(user.value);
	}
	return user;
};

router.get('/user/search?:query', async (req, res) => {
	const result = await fusion.searchUsers(req._parsedUrl.query);
	if (result.error) {
		return res.status(400).send({ message: 'Error fetching user' });
	}
	let users = [];
	for (let i = 0; i < result.json.length; i++) {
		let user = result.json[i];
		let { id, email, data, registrations, memberships, fullName, imageUrl, insertInstant, lastLoginInstant } = user;
		let { displayName, type, study, alumni, employee, discord } = data || {};
		let groupIds = memberships?.map((element) => element.groupId);
		let applicationIds = registrations?.map((element) => element.applicationId);
		let roles = registrations?.flatMap((element) => {
			if (element.roles) return element.roles;
		});
		fullName = fullName || displayName;
		displayName = displayName || fullName;
		imageUrl = imageUrl || DEFAULT_PROFILE_IMAGE;
		users.push({
			id,
			email,
			fullName,
			name: displayName,
			imageUrl,
			type,
			study,
			alumni,
			employee,
			discordUsername: discord?.username,
			isDiscordMember: discord?.isMember,
			insertInstant,
			lastLoginInstant,
			groupIds,
			applicationIds,
			roles
		});
	}
	return res.send({ users });
});

router.get('/user/:id', async (req, res) => {
	const user = await fusion.getUser(req.params.id);
	if (res.status == 404 || user.json?.verified === false) {
		return res.status(404).send({ message: 'User not found' });
	}
	if (user.error) {
		return res.status(400).send({ message: 'Error fetching user' });
	}
	let { id, email, data, fullName, imageUrl } = user.json;
	let { displayName, type, study, alumni, employee, discord } = data || {};
	fullName = fullName || displayName;
	displayName = fullName || fullName;
	imageUrl = imageUrl || DEFAULT_PROFILE_IMAGE;
	const discordUsername = discord?.username;

	let self = undefined;
	if (req.user?.id == id) {
		self = { isDiscordMember: discord?.isMember };
	}

	return res.send({ id, email, fullName, name: displayName, imageUrl, type, study, alumni, employee, discord: discordUsername, self });
});

router.put('/user', async (req, res) => {
	if (req.user) {
		return res.status(400).send({ message: 'You are already registered' });
	}
	const user = validate_user(req.body);
	if (user.error) {
		return res.status(400).send({ message: user.error.details[0].message });
	}

	const resp = await fusion.createUser({
		sendSetPasswordEmail: true,
		user: user.value
	});
	return res.status(resp.status).send(resp.client);
});

router.get('/discord/link', permission(), (req, res) => {
	res.redirect(discord.getOAuthLink());
});

router.get('/discord/callback', permission(), async (req, res) => {
	if (req.query.error == 'access_denied') {
		return res.redirect('/profil');
	}
	const id = await discord.getDiscordId(req, res, req.query.code);
	if (id && (await discord.updateDiscordInfo(req, res, id, req.user))) {
		res.redirect('/profil');
	}
});

router.post('/discord/refresh', permission(), async (req, res) => {
	const fUser = await fusion.getUser(req.user.id);
	if (fUser.error) {
		return res.status(500).send({ message: 'Error fetching userdata' });
	}
	if (!fUser.json?.data?.discord?.id) {
		return res.status(400).send({ message: 'User does not have a Discord account connected' });
	}
	if (await discord.updateDiscordInfo(req, res, fUser.json.data.discord.id, fUser)) {
		res.send({ message: 'Successfully refreshed Discord user data' });
	}
});

router.delete('/discord', permission(), async (req, res) => {
	const user = await fusion.getUser(req.user.id);

	if (user.json?.data?.discord?.id) {
		discord.removeRole(req, res, user.json.data.discord.id, true);
	}

	const mergeData = { data: { discord: null } };
	if (user.json.imageUrl == user.json?.data?.discord?.avatar) {
		mergeData.imageUrl = null;
	}
	const update = await fusion.updateUser(req.user.id, mergeData);
	if (update.error) {
		return res.status(500).send({ message: 'Error clearing Discord data' });
	}
	res.send({ message: 'Successfully cleared Discord data' });
});
