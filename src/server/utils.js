export const permission = (...roles) => {
	return (req, res, next) => {
		if (!roles.every(role => req.user.roles.includes(role))) {
			res.status(401).send({ error: true, msg: 'Permission denied' });
		} else {
			next();
		}
	}
}
