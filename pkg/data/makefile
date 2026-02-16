STAGE ?= dev


# To deploy your app and public folder
deploy: 
	gothicframework deploy --action deploy --stage $(STAGE)

delete: 
	gothicframework deploy --action delete --stage $(STAGE)

# Starts your Application in dev mode watching Templates, golang files and CSS files
dev: 
	gothicframework hot-reload


# Compiles Templates to generate equivalent golang files 
templ:
	gothicframework build	


# Generate CSS based on classes located on templ files
css:
	gothicframework css

# Generate CSS based on classes located on templ files in watch mode
optimize-images:
	gothicframework optimize-images


