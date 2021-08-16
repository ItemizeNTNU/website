export const permission = (...roles) => {
	return (req, res, next) => {
		if (!req.user) {
			return res.status(401).send({ message: 'You are not logged in' });
		}
		if (!roles.every((role) => req.user.roles.includes(role))) {
			return res.status(401).send({ message: 'Permission denied' });
		}
		next();
	};
};

export const removeEmptyObjects = (obj, cache) => {
	if (!Array.isArray(cache)) cache = [];
	if (cache.includes(obj)) return;
	cache.push(obj);
	for (let k of Object.keys(obj)) {
		if (typeof obj[k] == 'object') {
			if (Object.keys(obj[k]).length == 0) {
				delete obj[k];
			} else {
				removeEmptyObjects(obj[k], cache);
			}
		}
	}
};
