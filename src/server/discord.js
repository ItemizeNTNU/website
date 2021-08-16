import fetch from 'node-fetch';
import { fetchResource } from '../utils/api';
import fusion from './fusion';

const BOT_TOKEN = process.env.DISCORD_BOT_TOKEN;
const GUILD_ID = process.env.DISCORD_SERVER_ID;
const ROLE_ID = process.env.DISCORD_SERVER_MEMBER_ROLE_ID;
const CLIENT_ID = process.env.DISCORD_CLIENT_ID;
const CLIENT_SECRET = process.env.DISCORD_CLIENT_SECRET;
const API_BASE = 'https://discord.com/api/v8';

const discordFetch = async (path, options) => {
	if (!path.startsWith('http')) {
		path = `${API_BASE}${path}`;
	}
	options = { fetch, ...options };
	if (!options.noAuth) {
		options.headers = { ...options.headers };
		if (!options.headers.Authorization) {
			options.headers.Authorization = `Bot ${BOT_TOKEN}`;
		}
	}
	return await fetchResource(path, options);
};

export const getCallback = (req) => (req.headers['host'] == 'localhost:3000' ? 'http://localhost:3000' : 'https://itemize.no') + '/api/discord/callback';

export const getOAuthLink = (req) => {
	// https://discord.com/developers/docs/topics/oauth2#oauth2

	const DISCORD_API = `${API_BASE}/oauth2/authorize`;
	const RESPONSE_TYPE = 'code';
	const STATE = '123'; // TODO: implement and validate state to prevent CSRF (though risk is quite low)
	const SCOPE = encodeURIComponent(['identify'].join(' '));
	const CALLBACK = getCallback(req);
	const PROMPT = 'consent';
	return `${DISCORD_API}?response_type=${RESPONSE_TYPE}&client_id=${CLIENT_ID}&scope=${SCOPE}&state=${STATE}&redirect_uri=${CALLBACK}&prompt=${PROMPT}`;
};

export const getDiscordId = async (req, res, bearerCode) => {
	// https://discord.com/developers/docs/topics/oauth2#oauth2

	if (!bearerCode) {
		res.status(400).send({ message: 'Discord Auth Code is missiong' });
		return false;
	}

	const CALLBACK = getCallback(req);
	const reqAccessData = {
		client_id: CLIENT_ID,
		client_secret: CLIENT_SECRET,
		grant_type: 'authorization_code',
		code: bearerCode,
		redirect_uri: CALLBACK
	};
	const resAccess = await discordFetch('/oauth2/token', { method: 'POST', fetch, urlData: reqAccessData, noAuth: true });
	if (resAccess.error || !resAccess.json?.access_token) {
		res.status(400).send({ message: 'Unable to verify code token.' });
		return false;
	}

	const { access_token } = resAccess.json;
	// PATCH /users/@me https://discord.com/developers/docs/resources/user#modify-current-user
	const user = await discordFetch('/users/@me', { fetch, headers: { Authorization: `Bearer ${access_token}` } });
	if (!user.json?.id) {
		res.status(400).send({ message: 'Unable to fetch user data with provided code token.' });
		return false;
	}

	return user.json.id;
};

export const getGuildUser = async (req, res, discordId) => {
	// GET /guilds/{guild.id}/members https://discord.com/developers/docs/resources/guild#list-guild-members
	const guildUser = await discordFetch(`/guilds/${GUILD_ID}/members/${discordId}`);
	if (guildUser.error || !guildUser.json) {
		return null;
	}
	return guildUser.json;
};

export const getUser = async (req, res, discordId) => {
	// GET /users/{user.id} https://discord.com/developers/docs/resources/user#get-user
	const user = await discordFetch(`/users/${discordId}`);
	if (user.error || !user.json) {
		console.error('Error fetching discord profile for id: ' + discordId, user);
		res.status(500).send({ message: 'Error fetching Discord profile' });
		return false;
	}
	return user.json;
};

export const addRole = async (_, res, discordId) => {
	// PUT /guilds/{guild.id}/members/{user.id}/roles/{role.id} https://discord.com/developers/docs/resources/guild#add-guild-member-role
	const addRole = await discordFetch(`/guilds/${GUILD_ID}/members/${discordId}/roles/${ROLE_ID}`, { method: 'PUT' });
	if (addRole.error) {
		console.log('Error adding discord role to user', discordId, addRole);
		res.status(500).send('Error updating your Discord role');
		return false;
	}
	return addRole.json;
};

export const removeRole = async (req, res, discordId, surpressError = false) => {
	// DELETE /guilds/{guild.id}/members/{user.id}/roles/{role.id} https://discord.com/developers/docs/resources/guild#remove-guild-member-role
	const removeRole = await discordFetch(`/guilds/${GUILD_ID}/members/${discordId}/roles/${ROLE_ID}`, { method: 'DELETE' });
	if (removeRole.error && !surpressError) {
		console.error('Error removing discord server role for user:', req.user, removeRole);
		res.status(500).send({ message: 'Error removing Discord server role' });
		return false;
	}
	return true;
};

export const updateDiscordInfo = async (req, res, id, fusionUser) => {
	const discordUser = await getUser(req, res, id);
	if (!discordUser) return;
	let { username, avatar, discriminator } = discordUser;
	username = `${username}#${discriminator}`;
	avatar = 'https://cdn.discordapp.com/' + (avatar ? `avatars/${id}/${avatar}.png?size=512` : `embed/avatars/${discriminator % 5}.png`);

	const guildUser = await getGuildUser(req, res, id);
	const isMember = Boolean(guildUser?.user);

	if (isMember) {
		addRole(req, res, id);
	}

	const mergeData = { data: { discord: { id, username, avatar, isMember } }, imageUrl: avatar };
	if (!fusionUser.imageUrl || fusionUser.imageUrl == fusionUser.data?.discord?.avatar) {
		mergeData.imageUrl = avatar;
	}
	const userUpdate = await fusion.updateUser(req.user.id, mergeData);
	if (userUpdate.error) {
		console.error('Error updating discord information:', userUpdate);
		res.status(500).send({ message: 'Error updating user information.' });
		return false;
	}
	return true;
};

export default { addRole, removeRole, updateDiscordInfo, getDiscordId, getUser, getOAuthLink, getGuildUser };
