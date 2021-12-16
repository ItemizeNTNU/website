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
