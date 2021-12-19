import express from 'express';
import fusion from '../server/fusion';

export const router = express.Router();

router.get('/group', async (req, res) => {
	const result = await fusion.getAllGroups();

	if (result.error) {
		return res.status(400).send({ message: 'Error fetching groups' });
	}
	let jsonResult = result.json;
	let groups = {};
	for (let i = 0; i < jsonResult.length; i++) {
		let group = jsonResult[i];
		let { id, name, roles } = group;

		let newRoles = {};
		Object.keys(roles).forEach((k) => {
			newRoles[k] = roles[k].map((r) => (r = { id: r.id, name: r.name }));
		});
		roles = newRoles;

		groups[id] = { name, roles };
	}
	return res.send({ groups });
});

router.post('/group/member', async (req, res) => {
	const result = await fusion.addUsersToGroup(req.body.members);
	if (result.error) {
		return res.status(400).send({ message: 'Error adding user to group' });
	}
	return res.send({ result });
});

router.delete('/group/member', async (req, res) => {
	const result = await fusion.removeUsersFromGroup(req.body.members);
	if (result.error) {
		return res.status(400).send({ message: 'Error removing user from group' });
	}
	return res.send({ result });
});
