<script context="module">
	import api from '../utils/api';

	export async function preload(page, session) {
		if (!session.user) {
			this.redirect(302, '/login');
		} else if (!session.user?.roles.includes('Styret')) {
			this.redirect(401, '/');
		}
		const query = 'queryString=*';
		const dbUsers = await api.searchUsers(query, { fetch: this.fetch });
		const dbApplications = await api.getAllApplications({ fetch: this.fetch });
		const dbGroups = await api.getAllGroups({ fetch: this.fetch });
		return { users: dbUsers.json.users, applications: dbApplications.json.applications, groups: dbGroups.json.groups };
	}
</script>

<script>
	import { getModal } from '../components/PopupModal.svelte';
	import EditUserPopup from '../components/EditUserPopup.svelte';
	import AdminPanelTable from '../components/AdminPanelTable.svelte';
	import date from '../utils/date';
	import FaEdit from 'svelte-icons/fa/FaEdit.svelte';
	import MdAddBox from 'svelte-icons/md/MdAddBox.svelte';
	export let users;
	export let applications;
	export let groups;

	let attributeToEdit = '';
	let editType = '';
	let selectedCols = ['fullName', 'discordName', 'email', 'type'];

	const COLUMNS = {
		fullName: {
			key: 'fullName',
			title: 'Navn',
			value: (r) => r.fullName,
			sortable: true
		},
		discordName: {
			key: 'discordName',
			title: 'Discord-navn',
			value: (r) => r.discordUsername || '',
			sortable: true
		},
		email: {
			key: 'email',
			title: 'E-post',
			value: (r) => r.email,
			sortable: true
		},
		type: {
			key: 'type',
			title: 'Medlemstype',
			value: (r) => r.type || '',
			sortable: true
		},
		displayName: {
			key: 'displayName',
			title: 'Visningsnavn',
			value: (r) => r.displayName || '',
			edit: '',
			editConfirm: (v) => {
				return { data: { displayName: v } };
			}
		},
		study_program: {
			key: 'program',
			title: 'Studieretning',
			value: (r) => r.study?.program || '',
			edit: '',
			editConfirm: (v) => {
				return { data: { study: { program: v } } };
			}
		},
		study_year: {
			key: 'year',
			title: 'Årstrinn',
			value: (r) => r.study?.year || '',
			edit: [1, 2, 3, 4, 5, 6, 7, 8],
			editConfirm: (v) => {
				return { data: { study: { year: v } } };
			}
		},
		groups: {
			key: 'groupIds',
			title: 'Gruppe',
			title_plural: 'Grupper',
			value: (r) => r.groupIds?.map((id) => groups[id]?.name),
			add: (r) =>
				Object.keys(groups)
					.filter((id) => !r.groupIds?.includes(id))
					?.map((id) => groups[id].name)
		},
		applications: {
			key: 'applicationRoles',
			title: 'Applikasjon',
			title_plural: 'Applikasjoner',
			value: (r) => r.applicationRoles?.map((a) => applications[a.id]?.name),
			add: (r) =>
				Object.keys(applications)
					.filter((id) => !r.applicationRoles?.find((a) => a.id == id))
					?.map((id) => applications[id].name)
		}
	};
	const simpleFilter = {
		title: 'Søk etter brukere (navn, discord-navn og epost): ',
		filter: (r, v) => [r.name, r.discordUsername, r.email].find((e) => e?.toLocaleLowerCase().indexOf(v.toLocaleLowerCase()) >= 0)
	};
	const advancedFilter = [
		{ name: 'Navn: ', default: '', filter: (r, v) => r.fullName.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'Visningsnavn: ', default: '', filter: (r, v) => r.displayName.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'Discord-navn: ', default: '', filter: (r, v) => (r.discordUsername || '').toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'E-post: ', default: '', filter: (r, v) => r.email.toLocaleLowerCase().indexOf(v?.toLocaleLowerCase()) >= 0 },
		{ name: 'Registrert før: ', default: '', isDate: true, filter: (r, v) => r.insertInstant <= (new Date(v).getTime() || r.insertInstant) },
		{ name: 'Registrert etter: ', default: '', isDate: true, filter: (r, v) => r.insertInstant >= (new Date(v).getTime() || 0) },
		{ name: 'Medlemstype: ', default: [], values: ['alumni', 'employee', 'student'], filter: (r, v) => v?.includes(r.type) || v?.length == 0 },
		{
			name: 'Gruppe: ',
			default: [],
			values: Object.keys(groups).map((id) => groups[id].name),
			filter: (r, v) => r.groupIds?.find((id) => v?.includes(groups[id].name)) || v?.length == 0
		},
		{
			name: 'Applikasjon: ',
			default: [],
			values: Object.keys(applications).map((id) => applications[id].name),
			filter: (r, v) => r.applicationRoles?.find((app) => v?.includes(applications[app.id].name)) || v?.length == 0
		}
	];

	$: cols = selectedCols.map((key) => COLUMNS[key]);

	const editUser = (type, active) => {
		attributeToEdit = active;
		editType = type == 'edit' ? 'Endre' : 'Legg til';
		getModal('first').open();
	};
	async function updateValue(event) {
		let row = event.detail.row;
		if (event.detail.type == 'Endre') {
			const updatedUser = await api.patchUser(row.id, event.detail.user, { fetch: this.fetch });
			// update rows with updated user
			users = users.map((u) => (u.id == row.id ? updatedUser.json.user : u));
		}

		// TODO: Add confirmation of edit or error
	}
</script>

<svelte:head>
	<title>Admin panel</title>
</svelte:head>

<main>
	<div class="container">
		<div class="row">
			<div class="main-info">
				<h2>Totalt antall medlemmer: {users.length}</h2>
				<span>Studenter: {users.filter((e) => e.type == 'student').length}</span>
				<span>Alumni: {users.filter((e) => e.type == 'alumni').length}</span>
				<span>Ansatte: {users.filter((e) => e.type == 'employee').length}</span>
			</div>
			<br />
			<AdminPanelTable
				columns={cols}
				rows={users}
				showExpandIcon={true}
				expandRowKey="fullName"
				iconExpand="⌄"
				iconExpanded="⌃"
				simpleFilterOptions={simpleFilter}
				advancedFilterOptions={advancedFilter}
			>
				<div slot="expanded" let:row class="user-info">
					<p><b>Fullt Navn: </b>{row.fullName}</p>
					<p><b>Visningsnavn: </b>{row.displayName} <span on:click={() => editUser('edit', COLUMNS['displayName'])}><FaEdit /></span></p>
					<p><b>E-post: </b>{row.email}</p>
					<p><b>Medlemstype: </b>{row.type || ''}</p>
					{#if row.type == 'student' || row.type == 'alumni'}
						<p><b>Studieretning: </b>{row.study.program} <span on:click={() => editUser('edit', COLUMNS['study_program'])}><FaEdit /></span></p>
						{#if row.type == 'student'}
							<p><b>Årstrinn: </b>{row.study.year} <span on:click={() => editUser('edit', COLUMNS['study_year'])}><FaEdit /></span></p>
						{:else}
							<p><b>Medlemsår: </b>{row.alumni.joinYear}</p>
						{/if}
					{:else if row.type == 'employee'}
						<p><b>Title: </b>{row.employee.title}</p>
						<p />
					{/if}
					<div class="user-roles">
						<h4><b>Grupper</b><span on:click={() => editUser('add', COLUMNS['groups'])}><MdAddBox /></span></h4>
						<hr />
						{#if row?.groupIds}
							<div class="role-info">
								<p><b>Navn:</b></p>
								<p><b>Tilgang til applikasjoner:</b></p>
								{#each row.groupIds as id}
									<p>{groups[id]?.name}</p>
									<p>
										{Object.keys(groups[id]?.roles)
											.map((e) => applications[e].name)
											.join(', ')}
									</p>
								{/each}
							</div>
						{:else}
							<p><i>Ikke medlem av noen grupper</i></p>
						{/if}
					</div>
					<div class="user-roles">
						<h4><b>Applikasjoner</b><span on:click={() => editUser('add', COLUMNS['applications'])}><MdAddBox /></span></h4>
						<hr />
						{#if row?.applicationRoles}
							<div class="role-info">
								<span><b>Navn:</b></span><span><b>Registrert med roller:</b></span>
								{#each row.applicationRoles as application}
									<p>{applications[application.id]?.name}</p>
									<p>{application.roles.join(', ')}</p>
								{/each}
							</div>
						{:else}
							<p><i>Ikke tilgang til noen applikasjoner</i></p>
						{/if}
					</div>
					<p><b>Bruker opprettet: </b>{date.nicePrintDate(new Date(row.insertInstant))}</p>
					<p><b>Sist logget in: </b>{date.nicePrintDate(new Date(row.lastLoginInstant))}</p>
					<EditUserPopup {attributeToEdit} {row} type={editType} on:confirm={updateValue} />
				</div>
			</AdminPanelTable>
		</div>
	</div>
</main>

<style>
	.main-info {
		display: grid;
		grid-template-columns: 40% 20% 20% 20%;
		align-items: top;
		padding: 0.6em;
	}
	.user-info {
		display: grid;
		grid-template-columns: 50% 50%;
		align-items: top;
		padding: 0.6em;
	}
	.user-info * {
		margin: 0.1em 0;
	}
	.user-info p,
	div {
		width: 100%;
	}
	hr {
		display: block;
		height: 1px;
		border: 0;
		width: 70%;
		border-top: 1px solid #ccc;
		margin: 1em 0;
		padding: 0;
	}
	.user-roles {
		padding: 5px;
	}
	.user-roles p {
		padding: 5px;
	}
	.role-info {
		display: grid;
		grid-template-columns: 25% 75%;
		align-items: top;
		margin: 0;
		padding: 0;
	}
	.role-info p {
		width: 100%;
		padding: 0px;
		padding-left: 5px;
	}
	.user-info > p > span,
	.user-roles > h4 > span {
		display: inline-block;
		height: 0.75em;
		vertical-align: sub;
		margin-left: 4px;
		color: #fff;
		transition: 0.4s linear;
	}
	.user-info > p > span:hover,
	.user-roles > h4 > span:hover {
		color: var(--green-2);
	}
</style>
