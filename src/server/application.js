import express from 'express';
import fusion from '../server/fusion';

export const router = express.Router();

router.get('/application', async (req, res) => {
	const result = await fusion.getAllApplications();

	if (result.error) {
		return res.status(400).send({ message: 'Error fetching applications' });
	}
	let jsonResult = result.json;
	let applications = [];
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
		applications.push({ id, name, roles });
	}
	return res.send({ applications });
});
