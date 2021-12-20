import express from 'express';
import fusion from '../server/fusion';

export const router = express.Router();

router.get('/application', async (req, res) => {
	const result = await fusion.getAllApplications();

	if (result.error) {
		return res.status(400).send({ message: 'Error fetching applications' });
	}
	let jsonResult = result.json;
	let applications = {};
	for (let i = 0; i < jsonResult.length; i++) {
		let application = jsonResult[i];
		let { id, name, roles } = application;

		roles =
			roles?.map(
				(r) =>
					(r = {
						name: r.name,
						description: r.description,
						id: r.id
					})
			) || [];
		applications[id] = { name, roles };
	}
	return res.send({ applications });
});

router.patch('/user/registration/:id/:applicationId', async (req, res) => {
	const result = await fusion.patchUserRegistration(req.params.id, req.params.applicationId, req.body.registration);
	if (result.error) {
		return res.status(400).send({ message: 'Error adding roles to user' });
	}
	return res.send(result.json);
});

router.post('/user/registration/:id', async (req, res) => {
	const result = await fusion.addUserRegistration(req.params.id, req.body.registration);
	if (result.error) {
		return res.status(400).send({ message: 'Error adding roles to user' });
	}
	return res.send(result.json);
});

router.put('/user/registration/:id/:applicationId', async (req, res) => {
	const result = await fusion.putUserRegistration(req.params.id, req.params.applicationId, req.body.registration);
	if (result.error) {
		return res.status(400).send({ message: 'Error adding roles to user' });
	}
	return res.send(result.json);
});

router.delete('/user/registration/:id/:applicationId', async (req, res) => {
	const result = await fusion.deleteUserRegistration(req.params.id, req.params.applicationId);
	if (result.error) {
		return res.status(400).send({ message: 'Error deleting user registration' });
	}
	return res.send(result);
});
