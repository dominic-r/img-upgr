Image Upgrader:

This quick tool is ran in CI and allows me to update the tags of not-exactly-semver deps without wasting time on RenovateBot custom rules.

This is how it works:

1) Scans through docker-compose.yml files of env IMG_UPGR_SCANDIR to find `image:` lines;
2) Extracts all images and attempts to divide them in prefix/suffix and semver.
3) Then, it makes a request to the Docker Hub api to get all tags and finds an updated one meeting the extracted image format (e.g. `apache-2.34.0`)
4) For each updated file, a new branch is created and a merge request is pushed to Gitlab.

Required environment variables:

IMG_UPGR_SCANDIR - The (relative or full) directory to scan composes in 
IMG_UPGR_GL_USER - Gitlab bot username
IMG_UPGR_GL_TOKEN - Personal access token of the gitlab bot 
IMG_UPGR_GL_REPO - Repository URL of the destination repo. Is used when cloning the repository and when pushing merge requests to it. We don't need the project id as you can use /api/v4/projects/group%2Fuser/whatever instead of the ID
